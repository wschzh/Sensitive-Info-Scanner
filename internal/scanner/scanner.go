// Package scanner 实现敏感信息扫描核心：流式文件遍历、worker pool 并发提取与匹配、
// 结果内存上限（优先级保留）、并发安全的分页/增量查询与统计。
package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charlievieth/fastwalk"

	"sensitivescanner/internal/extract"
	"sensitivescanner/internal/patterns"
	"sensitivescanner/internal/types"
)

const (
	defaultMaxFileSize    int64 = 10 * 1024 * 1024
	defaultMaxResults           = 100000 // 内存保留结果上限（约 50-100MB），超出按优先级丢弃低级别
	maxWorkers                  = 64
	walkBuffer                  = 1024
	perFileTimeout              = 120 * time.Second // 单文件扫描超时：防个别卡死文件拖垮 worker
	maxMatchesPerLine           = 5000              // 每行每 pattern 最多记录匹配数（防 minified 巨行拖慢）
	lineContextPad              = 80                // LineContent 在匹配位置左右各取的字节数
	minifiedLineThreshold       = 4096              // 单行超过此字节数视为压缩/编译产物，跳过（噪音）
)

const (
	ProfileNormal       = "normal"
	ProfileFullDiskFast = "full_disk_fast"
	ProfileDeep         = "deep"
)

const (
	WalkEngineStd      = "std"
	WalkEngineFastwalk = "fastwalk"
)

const (
	skipExcludedDir     = "excluded_dir"
	skipExcludedPath    = "excluded_path"
	skipExcludedExt     = "excluded_ext"
	skipUnsupportedExt  = "unsupported_ext"
	skipTooLarge        = "too_large"
	skipNonRegular      = "non_regular"
	skipProfileDisabled = "profile_disabled"
)

// ResultFilter 分页查询的筛选条件（服务端筛选，避免前端全量驻留）。
type ResultFilter struct {
	Level   types.Level // 空 = 全部级别
	Type    string      // 空 = 全部类型（PatternName）
	Keyword string      // 在 file_path/matched_text/line_content 中忽略大小写搜索
}

// Config 扫描配置。零值字段走合理默认，向后兼容。
type Config struct {
	MaxFileSize         int64         // 最大文件大小（字节），0 = 默认 10MB
	ScanLevels          []types.Level // 要扫描的级别，空 = 全部
	Workers             int           // 并发 worker 数，0 = NumCPU（封顶 64）
	MaxResults          int           // 内存保留的结果上限，0 = 默认 100000
	KeepLevel           types.Level   // >= 该级别的结果永远保留（不受 MaxResults 限制），空 = Medium
	ScanProfile         string        // normal/full_disk_fast/deep，空 = normal
	WalkEngine          string        // std/fastwalk，空 = std
	ExcludePathKeywords []string      // 路径分段关键词排除，空 = 默认 log/logs/cache/temp/tmp
	DisableImages       bool          // 跳过图片/OCR 慢路径
}

// Scanner 敏感信息扫描器（并发安全，供 CLI 与 Web GUI 共用）。
type Scanner struct {
	cfg      Config
	patterns []patterns.Pattern
	single   []patterns.Pattern // 单行模式（逐行匹配）
	cross    []patterns.Pattern // 跨行模式（整段匹配，如私钥）

	mu      sync.Mutex // 保护 results / stats / cancel
	results []types.ScanResult
	stats   types.ScanStatistics
	cancel  context.CancelFunc

	pmu     sync.Mutex // 保护 current
	current string

	scanned    atomic.Int64 // 已完成扫描的文件数
	discovered atomic.Int64 // walker 已发现的文件数（扫描中实时增长）
}

// New 创建扫描器。
func New(cfg Config) *Scanner {
	cfg = normalizeConfig(cfg)
	ps := patterns.All()
	if len(cfg.ScanLevels) > 0 {
		ps = patterns.ByLevel(cfg.ScanLevels...)
	}
	single, cross := patterns.SplitCrossLine(ps)
	return &Scanner{
		cfg:      cfg,
		patterns: ps,
		single:   single,
		cross:    cross,
		stats:    types.NewScanStatistics(),
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.ScanProfile == "" {
		cfg.ScanProfile = ProfileNormal
	}
	if cfg.WalkEngine == "" {
		cfg.WalkEngine = WalkEngineStd
	}
	if cfg.ScanProfile == ProfileFullDiskFast {
		if len(cfg.ScanLevels) == 0 {
			cfg.ScanLevels = []types.Level{types.Critical, types.High}
		}
		cfg.DisableImages = true
	}
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = defaultMaxFileSize
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = defaultMaxResults
	}
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU()
	}
	if cfg.Workers > maxWorkers {
		cfg.Workers = maxWorkers
	}
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	if cfg.KeepLevel == "" {
		cfg.KeepLevel = types.Medium
	}
	if len(cfg.ExcludePathKeywords) == 0 {
		cfg.ExcludePathKeywords = []string{"log", "logs", "cache", "temp", "tmp"}
	}
	for i, kw := range cfg.ExcludePathKeywords {
		cfg.ExcludePathKeywords[i] = strings.ToLower(strings.TrimSpace(kw))
	}
	return cfg
}

// Results 返回内存内结果的拷贝（受 MaxResults 上限）。
func (s *Scanner) Results() []types.ScanResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.ScanResult, len(s.results))
	copy(out, s.results)
	return out
}

// ResultsPage 按筛选条件分页返回结果，total 为筛选后的总条数。
func (s *Scanner) ResultsPage(offset, limit int, f ResultFilter) ([]types.ScanResult, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.filterLocked(f)
	total := len(filtered)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	if limit <= 0 {
		limit = 1000
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := make([]types.ScanResult, end-offset)
	copy(out, filtered[offset:end])
	return out, total
}

// ResultsSince 返回序号 fromSeq 之后新增的结果（扫描中增量拉取），nextSeq 为当前结果总数。
func (s *Scanner) ResultsSince(fromSeq int) ([]types.ScanResult, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fromSeq < 0 {
		fromSeq = 0
	}
	if fromSeq > len(s.results) {
		fromSeq = len(s.results)
	}
	out := make([]types.ScanResult, len(s.results)-fromSeq)
	copy(out, s.results[fromSeq:])
	return out, len(s.results)
}

// RemoveResultsForPaths 从内存结果集中移除指定文件路径对应的命中，并同步修正展示统计。
// 删除文件后调用，避免后端保留旧扫描快照导致刷新页面时已删除文件重新出现。
func (s *Scanner) RemoveResultsForPaths(paths map[string]bool) int {
	if len(paths) == 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	kept := s.results[:0]
	removed := 0
	for _, r := range s.results {
		if paths[r.FilePath] {
			removed++
			if s.stats.TotalIssues > 0 {
				s.stats.TotalIssues--
			}
			if s.stats.IssuesByLevel[r.Level] > 0 {
				s.stats.IssuesByLevel[r.Level]--
			}
			continue
		}
		kept = append(kept, r)
	}
	s.results = kept

	files := make(map[string]bool)
	for _, r := range s.results {
		files[r.FilePath] = true
	}
	s.stats.FilesWithIssues = len(files)
	return removed
}

// filterLocked 调用方须持 s.mu。无筛选条件时直接返回 results 引用（caller 再按页拷贝）。
func (s *Scanner) filterLocked(f ResultFilter) []types.ScanResult {
	if f.Level == "" && f.Type == "" && f.Keyword == "" {
		return s.results
	}
	kw := strings.ToLower(f.Keyword)
	out := make([]types.ScanResult, 0)
	for _, r := range s.results {
		if f.Level != "" && r.Level != f.Level {
			continue
		}
		if f.Type != "" && r.PatternName != f.Type {
			continue
		}
		if kw != "" {
			hay := strings.ToLower(r.FilePath + " " + r.MatchedText + " " + r.LineContent)
			if !strings.Contains(hay, kw) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// Stats 返回统计快照（含 map，调用方只读勿改）。
func (s *Scanner) Stats() types.ScanStatistics {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := s.stats
	stats.IssuesByLevel = copyLevelMap(s.stats.IssuesByLevel)
	stats.TruncatedByLevel = copyLevelMap(s.stats.TruncatedByLevel)
	stats.SkippedByReason = copyStringMap(s.stats.SkippedByReason)
	return stats
}

// Progress 返回 (已扫描, 已发现, 当前文件)。total 扫描中实时增长，结束时等于 TotalFiles。
func (s *Scanner) Progress() (scanned, total int, current string) {
	s.pmu.Lock()
	current = s.current
	s.pmu.Unlock()
	return int(s.scanned.Load()), int(s.discovered.Load()), current
}

// Stop 请求停止扫描（取消当前扫描的 context，worker/walker 随即退出）。
func (s *Scanner) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
}

func (s *Scanner) reset() {
	s.mu.Lock()
	s.results = nil
	s.stats = types.NewScanStatistics()
	s.cancel = nil
	s.mu.Unlock()
	s.scanned.Store(0)
	s.discovered.Store(0)
	s.pmu.Lock()
	s.current = ""
	s.pmu.Unlock()
}

// ScanSingle 同步扫描单个文件（无并发，保留简单路径）。
func (s *Scanner) ScanSingle(path string) {
	s.reset()
	start := time.Now()
	if ok, reason := s.shouldScan(path); !ok {
		s.addSkipped(false, reason)
		s.mu.Lock()
		s.stats.TotalFiles = 0
		s.stats.ScanDuration = time.Since(start).Seconds()
		s.mu.Unlock()
		return
	}
	s.discovered.Store(1)
	s.mu.Lock()
	s.stats.TotalFiles = 1
	s.mu.Unlock()

	fr := s.ScanFile(path)
	s.scanned.Add(1)

	s.mu.Lock()
	s.stats.ScannedFiles = 1
	if len(fr) > 0 {
		s.stats.FilesWithIssues = 1
		for _, r := range fr {
			s.appendResultLocked(r)
		}
	}
	s.stats.ScanDuration = time.Since(start).Seconds()
	s.mu.Unlock()
}

// ScanDirectory 流式遍历 + worker pool 并发扫描。
// producer(walkDir) → fileCh → N worker → resultCh → 单写者 aggregator(本 goroutine)。
func (s *Scanner) ScanDirectory(root string, recursive bool) {
	s.reset()
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()
	}()

	start := time.Now()

	fileCh := make(chan string, walkBuffer)
	resultCh := make(chan []types.ScanResult, s.cfg.Workers)

	// producer：流式遍历，路径不再入驻留切片
	go func() {
		defer close(fileCh)
		s.walkDir(ctx, root, recursive, fileCh)
	}()

	// worker pool
	var wg sync.WaitGroup
	for i := 0; i < s.cfg.Workers; i++ {
		wg.Add(1)
		go s.scanWorker(ctx, fileCh, resultCh, &wg)
	}
	// closer：所有 worker 退出后关闭 resultCh
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// aggregator：单一写者，串行 append results + 累加 stats（无需分片锁/原子 map）
	for fr := range resultCh {
		s.scanned.Add(1)
		s.mu.Lock()
		s.stats.ScannedFiles++
		if len(fr) > 0 {
			s.stats.FilesWithIssues++
			for _, r := range fr {
				s.appendResultLocked(r)
			}
		}
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.stats.TotalFiles = int(s.discovered.Load())
	s.stats.ScanDuration = time.Since(start).Seconds()
	s.mu.Unlock()
	s.pmu.Lock()
	s.current = ""
	s.pmu.Unlock()
}

// scanFileTimeout 对单文件扫描加超时与 panic 保护：超时或异常则跳过该文件，
// 避免 worker 因个别卡死文件（特殊文件 ReadFile 阻塞、损坏图片 OCR 卡住等）永久停滞。
// 超时后底层 goroutine 会在自身操作（OCR 60s / 常规 ReadFile）完成后自行退出，不会永久泄漏。
func (s *Scanner) scanFileTimeout(ctx context.Context, path string) []types.ScanResult {
	type result struct{ r []types.ScanResult }
	ch := make(chan result, 1)
	go func() {
		defer func() { _ = recover() }() // 防个别文件触发 panic 拖垮整个 worker
		ch <- result{s.ScanFile(path)}
	}()
	select {
	case r := <-ch:
		return r.r
	case <-time.After(perFileTimeout):
		return nil
	case <-ctx.Done():
		return nil
	}
}

func (s *Scanner) scanWorker(ctx context.Context, fileCh <-chan string, resultCh chan<- []types.ScanResult, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case path, ok := <-fileCh:
			if !ok {
				return
			}
			s.pmu.Lock()
			s.current = path
			s.pmu.Unlock()
			fr := s.scanFileTimeout(ctx, path)
			select {
			case resultCh <- fr:
			case <-ctx.Done():
				return
			}
		}
	}
}

// walkDir 流式遍历目录，每个可扫描文件 discovered++ 并发送到 fileCh；响应 ctx 停止。
func (s *Scanner) walkDir(ctx context.Context, root string, recursive bool, fileCh chan<- string) {
	send := func(path string) bool {
		s.discovered.Add(1)
		select {
		case fileCh <- path:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if recursive {
		walkFn := func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if ctx.Err() != nil {
				return filepath.SkipDir
			}
			if d.IsDir() {
				if path != root && patterns.IsExcludedDir(d.Name()) {
					s.addSkipped(true, skipExcludedDir)
					return filepath.SkipDir
				}
				if path != root && s.pathHasExcludedKeyword(path) {
					s.addSkipped(true, skipExcludedPath)
					return filepath.SkipDir
				}
				return nil
			}
			if ok, reason := s.shouldScan(path); ok {
				if !send(path) {
					return filepath.SkipDir
				}
			} else {
				s.addSkipped(false, reason)
			}
			return nil
		}
		if s.cfg.WalkEngine == WalkEngineFastwalk {
			_ = fastwalk.Walk(nil, root, walkFn)
			return
		}
		_ = filepath.WalkDir(root, walkFn)
	} else {
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			if ctx.Err() != nil {
				return
			}
			if e.IsDir() {
				continue
			}
			p := filepath.Join(root, e.Name())
			if ok, reason := s.shouldScan(p); ok {
				if !send(p) {
					return
				}
			} else {
				s.addSkipped(false, reason)
			}
		}
	}
}

// appendResultLocked 调用方须持 s.mu。统计始终累加；结果按优先级保留策略决定是否入内存：
// 未达 MaxResults 上限，或级别 >= KeepLevel（优先级数字越小越高）则永远保留；否则只计截断数。
func (s *Scanner) appendResultLocked(r types.ScanResult) {
	s.stats.TotalIssues++
	s.stats.IssuesByLevel[r.Level]++
	if len(s.results) < s.cfg.MaxResults || levelPriority(r.Level) <= levelPriority(s.cfg.KeepLevel) {
		s.results = append(s.results, r)
	} else {
		s.stats.TruncatedCount++
		s.stats.TruncatedByLevel[r.Level]++
	}
}

// levelPriority 级别优先级（1 最高，来自 types.LevelConfig）。
func levelPriority(l types.Level) int {
	return types.LevelConfig[l].Priority
}

// ScanFile 扫描单个文件，返回命中的结果。
func (s *Scanner) ScanFile(path string) []types.ScanResult {
	content, ok := s.readFile(path)
	if !ok || content == "" {
		return nil
	}
	if isMinified(content) {
		return nil // 跳过压缩/编译产物（minified JS/CSS 等），其中 URL/IP/数字多为字面量噪音
	}
	return s.matchContent(path, content)
}

// isMinified 判断是否为压缩/编译产物：存在单行超过 minifiedLineThreshold 字节。
// 这类文件（如 minified JS）扫出的 URL/IP/数字串多为代码字面量噪音，跳过以提高信噪比。
// 找到一行超阈值即提前返回，避免遍历整个大文件。
func isMinified(content string) bool {
	lineStart := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			if i-lineStart > minifiedLineThreshold {
				return true
			}
			lineStart = i + 1
		}
	}
	return len(content)-lineStart > minifiedLineThreshold // 末行（无尾换行）
}

// readFile 按扩展名分流提取文本：xlsx/docx/pdf 走 extract 包，其余走多编码文本读取。
func (s *Scanner) readFile(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	if patterns.IsExcludedFileName(filepath.Base(path)) || !patterns.IsScannableExt(ext) {
		return "", false
	}
	if fi, err := os.Stat(path); err == nil {
		if !fi.Mode().IsRegular() {
			s.addSkipped(false, skipNonRegular)
			return "", false // 跳过设备/管道/套接字等非常规文件，避免 ReadFile 永久阻塞
		}
		if s.cfg.MaxFileSize > 0 && fi.Size() > s.cfg.MaxFileSize {
			s.addSkipped(false, skipTooLarge)
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
		if s.cfg.DisableImages {
			s.addSkipped(false, skipProfileDisabled)
			return "", false
		}
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

// matchContent 匹配文本：单行模式对全文一次性匹配（比逐行 × N pattern 快得多，
// 尤其是行数多的大文件——避免「行数 × pattern 数」次 FindAllStringIndex 调用），
// 跨行模式（如私钥）整段匹配。LineContent 只取匹配附近（防止单行巨文件每条结果存整行）。
func (s *Scanner) matchContent(path, content string) []types.ScanResult {
	var results []types.ScanResult
	now := time.Now()
	lineStarts := computeLineStarts(content)
	lowerContent := ""
	digitCount := -1

	// 单行模式：全文一次性匹配，用二分把匹配偏移换算成行号
	for _, p := range s.single {
		if !patternCandidateMatch(content, &lowerContent, &digitCount, p) {
			continue
		}
		for _, loc := range p.RE().FindAllStringIndex(content, maxMatchesPerLine) {
			start, end := loc[0], loc[1]
			lineNum := sort.Search(len(lineStarts), func(i int) bool { return lineStarts[i] > start })
			if lineNum == 0 {
				lineNum = 1
			}
			ls := lineStarts[lineNum-1]
			le := end
			for le < len(content) && content[le] != '\n' {
				le++
			}
			results = append(results, types.ScanResult{
				FilePath:    path,
				PatternName: p.Name,
				Level:       p.Level,
				LineNumber:  lineNum,
				LineContent: contextAround(content[ls:le], start-ls, end-ls, lineContextPad),
				MatchedText: content[start:end],
				Timestamp:   now,
			})
		}
	}

	// 跨行模式：整段匹配，行号用匹配起点前的 \n 数计算，LineContent 截断
	for _, p := range s.cross {
		if !patternCandidateMatch(content, &lowerContent, &digitCount, p) {
			continue
		}
		for _, loc := range p.RE().FindAllStringIndex(content, -1) {
			lineNum := 1 + strings.Count(content[:loc[0]], "\n")
			results = append(results, types.ScanResult{
				FilePath:    path,
				PatternName: p.Name,
				Level:       p.Level,
				LineNumber:  lineNum,
				LineContent: truncateRunes(strings.TrimSpace(content[loc[0]:loc[1]]), 200),
				MatchedText: content[loc[0]:loc[1]],
				Timestamp:   now,
			})
		}
	}
	return results
}

func patternCandidateMatch(content string, lowerContent *string, digitCount *int, p patterns.Pattern) bool {
	if p.MinDigits > 0 {
		if digitCount != nil && *digitCount < 0 {
			*digitCount = countDigits(content)
		}
		if digitCount != nil && *digitCount < p.MinDigits {
			return false
		}
	}
	return patternHintsMatch(content, lowerContent, p.Hints, p.LowerHints())
}

func patternHintsMatch(content string, lowerContent *string, hints, lowerHints []string) bool {
	if len(hints) == 0 {
		return true
	}
	for i, h := range hints {
		if h == "" {
			continue
		}
		if strings.Contains(content, h) {
			return true
		}
		if lowerContent != nil && *lowerContent == "" {
			*lowerContent = strings.ToLower(content)
		}
		lowerHint := strings.ToLower(h)
		if i < len(lowerHints) {
			lowerHint = lowerHints[i]
		}
		if lowerContent != nil && strings.Contains(*lowerContent, lowerHint) {
			return true
		}
	}
	return false
}

func countDigits(s string) int {
	count := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			count++
		}
	}
	return count
}

// computeLineStarts 返回每行起始字节偏移（第 1 行恒为 0），供 matchContent 二分算行号。
// 临时分配约 8B/行（大文件扫完即释放），权衡速度——全文匹配比逐行快一两个数量级。
func computeLineStarts(content string) []int {
	starts := make([]int, 1, strings.Count(content, "\n")+1)
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// contextAround 返回匹配位置附近的行内容（左右各 pad 字节），两端用 … 标注截断。
// 关键：避免压缩成单行的巨文件（如 minified JS）让每条结果都存整行而内存爆炸。
func contextAround(line string, start, end, pad int) string {
	s := start - pad
	if s < 0 {
		s = 0
	}
	e := end + pad
	if e > len(line) {
		e = len(line)
	}
	var b strings.Builder
	if s > 0 {
		b.WriteString("…")
	}
	b.WriteString(strings.TrimSpace(line[s:e]))
	if e < len(line) {
		b.WriteString("…")
	}
	return b.String()
}

// truncateRunes 截断到最多 n 个 rune（跨行匹配段过长时用）。
func truncateRunes(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func (s *Scanner) shouldScan(path string) (bool, string) {
	if s.pathHasExcludedKeyword(path) {
		return false, skipExcludedPath
	}
	ext := strings.ToLower(filepath.Ext(path))
	if patterns.IsExcludedFileName(filepath.Base(path)) {
		return false, skipExcludedExt
	}
	if !patterns.IsScannableExt(ext) {
		return false, skipUnsupportedExt
	}
	return true, ""
}

func (s *Scanner) pathHasExcludedKeyword(path string) bool {
	if len(s.cfg.ExcludePathKeywords) == 0 {
		return false
	}
	for _, part := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		lower := strings.ToLower(part)
		for _, kw := range s.cfg.ExcludePathKeywords {
			if kw != "" && lower == kw {
				return true
			}
		}
	}
	return false
}

func (s *Scanner) addSkipped(isDir bool, reason string) {
	if reason == "" {
		reason = "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if isDir {
		s.stats.SkippedDirs++
	} else {
		s.stats.SkippedFiles++
	}
	s.stats.SkippedByReason[reason]++
}

func copyLevelMap(in map[types.Level]int) map[types.Level]int {
	out := make(map[types.Level]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyStringMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
