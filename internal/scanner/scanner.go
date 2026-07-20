// Package scanner 实现敏感信息扫描核心：文件遍历、按扩展名分流提取、逐行正则匹配、统计与进度。
package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sensitivescanner/internal/extract"
	"sensitivescanner/internal/patterns"
	"sensitivescanner/internal/types"
)

// Config 扫描配置。
type Config struct {
	MaxFileSize int64         // 最大文件大小（字节），0 表示用默认 10MB
	ScanLevels  []types.Level // 要扫描的级别，空表示全部
}

// Scanner 敏感信息扫描器（并发安全，供 CLI 与 Web GUI 共用）。
type Scanner struct {
	cfg      Config
	patterns []patterns.Pattern

	mu          sync.Mutex
	results     []types.ScanResult
	stats       types.ScanStatistics
	stopFlag    bool
	progress    int
	currentFile string
}

// New 创建扫描器。
func New(cfg Config) *Scanner {
	ps := patterns.All()
	if len(cfg.ScanLevels) > 0 {
		ps = patterns.ByLevel(cfg.ScanLevels...)
	}
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = 10 * 1024 * 1024
	}
	return &Scanner{
		cfg:      cfg,
		patterns: ps,
		stats:    types.NewScanStatistics(),
	}
}

// Results 返回结果的拷贝。
func (s *Scanner) Results() []types.ScanResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.ScanResult, len(s.results))
	copy(out, s.results)
	return out
}

// Stats 返回统计快照。
func (s *Scanner) Stats() types.ScanStatistics {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// Progress 返回 (百分比, 当前文件)。
func (s *Scanner) Progress() (int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress, s.currentFile
}

// Stop 请求停止扫描。
func (s *Scanner) Stop() {
	s.mu.Lock()
	s.stopFlag = true
	s.mu.Unlock()
}

func (s *Scanner) stopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopFlag
}

func (s *Scanner) reset() {
	s.mu.Lock()
	s.results = nil
	s.stats = types.NewScanStatistics()
	s.stopFlag = false
	s.progress = 0
	s.mu.Unlock()
}

// ScanSingle 扫描单个文件并更新统计。
func (s *Scanner) ScanSingle(path string) {
	s.reset()
	s.mu.Lock()
	s.stats.TotalFiles = 1
	s.mu.Unlock()

	start := time.Now()
	fr := s.ScanFile(path)

	s.mu.Lock()
	s.stats.ScannedFiles = 1
	if len(fr) > 0 {
		s.results = append(s.results, fr...)
		s.stats.FilesWithIssues = 1
		s.stats.TotalIssues = len(fr)
		for _, r := range fr {
			s.stats.IssuesByLevel[r.Level]++
		}
	}
	s.stats.ScanDuration = time.Since(start).Seconds()
	s.mu.Unlock()
}

// ScanDirectory 扫描目录（支持递归）。
func (s *Scanner) ScanDirectory(root string, recursive bool) {
	s.reset()

	start := time.Now()
	files := s.collectFiles(root, recursive)

	s.mu.Lock()
	s.stats.TotalFiles = len(files)
	s.mu.Unlock()

	if len(files) == 0 {
		s.mu.Lock()
		s.stats.ScanDuration = time.Since(start).Seconds()
		s.mu.Unlock()
		return
	}

	for i, f := range files {
		if s.stopped() {
			break
		}
		s.mu.Lock()
		s.progress = int(float64(i) / float64(len(files)) * 100)
		s.currentFile = f
		s.mu.Unlock()

		fr := s.ScanFile(f)
		s.mu.Lock()
		s.stats.ScannedFiles++
		if len(fr) > 0 {
			s.results = append(s.results, fr...)
			s.stats.FilesWithIssues++
			s.stats.TotalIssues += len(fr)
			for _, r := range fr {
				s.stats.IssuesByLevel[r.Level]++
			}
		}
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.stats.ScanDuration = time.Since(start).Seconds()
	s.progress = 100
	s.mu.Unlock()
}

// ScanFile 扫描单个文件，返回命中的结果。
func (s *Scanner) ScanFile(path string) []types.ScanResult {
	content, ok := s.readFile(path)
	if !ok || content == "" {
		return nil
	}
	return s.matchContent(path, content)
}

// readFile 按扩展名分流提取文本：xlsx/docx/pdf 走 extract 包，其余走多编码文本读取。
func (s *Scanner) readFile(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	if patterns.IsExcludedExt(ext) || !patterns.IsScannableExt(ext) {
		return "", false
	}
	if fi, err := os.Stat(path); err == nil {
		if s.cfg.MaxFileSize > 0 && fi.Size() > s.cfg.MaxFileSize {
			return "", false
		}
	}
	switch ext {
	case ".xlsx":
		c, err := extract.XLSX(path)
		return c, err == nil
	case ".docx":
		c, err := extract.DOCX(path)
		return c, err == nil
	case ".pdf":
		c, err := extract.PDF(path)
		return c, err == nil
	case ".jpg", ".jpeg", ".png", ".bmp", ".tiff", ".tif", ".gif", ".webp":
		c, err := extract.Image(path)
		return c, err == nil // 找不到 tesseract 时静默跳过
	default:
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		return decodeText(raw), true
	}
}

// matchContent 在全文上匹配所有已启用的模式，并用二分把匹配偏移换算成行号。
// 全文匹配（而非逐行）是为了能抓到跨行的私钥 -----BEGIN...-----END----- 等模式；
// 单行模式（手机/IP/URL 等）因字符集不含换行，行为与逐行一致。
func (s *Scanner) matchContent(path, content string) []types.ScanResult {
	var results []types.ScanResult
	now := time.Now()
	lineStarts := computeLineStarts(content)
	for _, p := range s.patterns {
		for _, loc := range p.RE().FindAllStringIndex(content, -1) {
			start, end := loc[0], loc[1]
			// 行号 = 第一个行起始偏移 > start 的位置（lineStarts[0]==0 对应第 1 行）
			lineNum := sort.Search(len(lineStarts), func(i int) bool { return lineStarts[i] > start })
			if lineNum == 0 {
				lineNum = 1
			}
			lineStart := lineStarts[lineNum-1]
			lineEnd := end
			for lineEnd < len(content) && content[lineEnd] != '\n' {
				lineEnd++
			}
			results = append(results, types.ScanResult{
				FilePath:    path,
				PatternName: p.Name,
				Level:       p.Level,
				LineNumber:  lineNum,
				LineContent: strings.TrimSpace(content[lineStart:lineEnd]),
				MatchedText: content[start:end],
				Timestamp:   now,
			})
		}
	}
	return results
}

// computeLineStarts 返回每行起始字节偏移（第 1 行恒为 0）。
func computeLineStarts(content string) []int {
	starts := []int{0}
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// collectFiles 收集待扫描文件，按 ExcludeDirs 剪枝。
func (s *Scanner) collectFiles(root string, recursive bool) []string {
	var files []string
	if recursive {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && patterns.IsExcludedDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if s.shouldScan(path) {
				files = append(files, path)
			}
			return nil
		})
	} else {
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			p := filepath.Join(root, e.Name())
			if s.shouldScan(p) {
				files = append(files, p)
			}
		}
	}
	return files
}

func (s *Scanner) shouldScan(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return patterns.IsScannableExt(ext) && !patterns.IsExcludedExt(ext)
}
