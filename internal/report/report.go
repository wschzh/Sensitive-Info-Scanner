// Package report 流式生成 text/json 扫描报告（写 io.Writer，不累积全量字符串）。
// 写文件时统一加 UTF-8 BOM，确保 Windows 记事本能正确显示中文。
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"sensitivescanner/internal/scanner"
	"sensitivescanner/internal/types"
)

// Format 报告格式。
type Format string

const (
	Text Format = "text"
	JSON Format = "json"
)

const sep = "================================================================================"

// GenerateTo 按 format 流式生成报告到 w（不累积全量字符串，适合 HTTP 响应/大结果集）。
func GenerateTo(s *scanner.Scanner, format Format, w io.Writer) error {
	if format == JSON {
		return genJSONTo(s, w)
	}
	return genTextTo(s, w)
}

// GenerateToFile 生成报告到文件；text 格式先写 UTF-8 BOM。
func GenerateToFile(s *scanner.Scanner, format Format, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if format == Text {
		if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			return err
		}
	}
	return GenerateTo(s, format, f)
}

// Generate 生成报告字符串（向后兼容；内部走 GenerateTo + buffer）。output 非空则写文件（带 BOM）。
func Generate(s *scanner.Scanner, format Format, output string) (string, error) {
	var buf strings.Builder
	if err := GenerateTo(s, format, &buf); err != nil {
		return "", err
	}
	content := buf.String()
	if output != "" {
		data := []byte(content)
		if format == Text {
			data = WithBOM(content)
		}
		if err := os.WriteFile(output, data, 0o644); err != nil {
			return content, err
		}
	}
	return content, nil
}

// WithBOM 在内容前加 UTF-8 BOM，返回可直接写入的字节切片。
func WithBOM(content string) []byte {
	return append([]byte{0xEF, 0xBB, 0xBF}, content...)
}

// genTextTo 流式生成文本报告：头部统计（含截断提示）+ 按扫描顺序的明细（不再 byLevel 二次分桶）。
func genTextTo(s *scanner.Scanner, w io.Writer) error {
	stats := s.Stats()
	fmt.Fprintf(w, "%s\n敏感信息扫描报告\n%s\n", sep, sep)
	fmt.Fprintf(w, "扫描时长: %.2f 秒\n", stats.ScanDuration)
	fmt.Fprintf(w, "总文件数: %d  已扫描: %d  有问题: %d  问题总数: %d",
		stats.TotalFiles, stats.ScannedFiles, stats.FilesWithIssues, stats.TotalIssues)
	if stats.TruncatedCount > 0 {
		fmt.Fprintf(w, "  (已截断 %d 条，仅保留高优先级)", stats.TruncatedCount)
	}
	if stats.SkippedFiles > 0 || stats.SkippedDirs > 0 {
		fmt.Fprintf(w, "\n已跳过: %d 个文件 / %d 个目录", stats.SkippedFiles, stats.SkippedDirs)
	}
	fmt.Fprintf(w, "\n\n问题级别分布:\n")
	for _, lv := range types.AllLevels {
		if c := stats.IssuesByLevel[lv]; c > 0 {
			fmt.Fprintf(w, "  %s: %d\n", types.LevelConfig[lv].Name, c)
		}
	}
	fmt.Fprintf(w, "\n")

	results := s.Results()
	if len(results) == 0 {
		fmt.Fprintf(w, "未发现敏感信息\n")
		return nil
	}
	for _, f := range s.ResultsByFile(scanner.ResultFilter{}) {
		count := fmt.Sprintf("%d", f.IssueCount)
		if f.IssueOverflow {
			count += "+"
		}
		fmt.Fprintf(w, "[%s] %s  问题:%s  类型:%s\n",
			types.LevelConfig[f.HighestLevel].Name, f.FilePath, count, strings.Join(f.PatternNames, "、"))
		for _, r := range f.Samples {
			content := cleanPrintable(r.LineContent)
			if len([]rune(content)) > 100 {
				content = string([]rune(content)[:100]) + "..."
			}
			fmt.Fprintf(w, "  - 第%d行  %s  匹配:%s  内容:%s\n",
				r.LineNumber, r.PatternName, cleanPrintable(r.MatchedText), content)
		}
	}
	return nil
}

// genJSONTo 流式编码 JSON（Encoder 直接写 w，不返回巨型 []byte）。
func genJSONTo(s *scanner.Scanner, w io.Writer) error {
	stats := s.Stats()
	data := map[string]any{
		"scan_duration": stats.ScanDuration,
		"statistics": map[string]any{
			"total_files":       stats.TotalFiles,
			"scanned_files":     stats.ScannedFiles,
			"skipped_files":     stats.SkippedFiles,
			"skipped_dirs":      stats.SkippedDirs,
			"skipped_by_reason": stats.SkippedByReason,
			"files_with_issues": stats.FilesWithIssues,
			"total_issues":      stats.TotalIssues,
			"issues_by_level":   stats.IssuesByLevel,
			"truncated_count":   stats.TruncatedCount,
		},
		"files":   s.ResultsByFile(scanner.ResultFilter{}),
		"results": s.Results(),
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(data)
}

// cleanPrintable 把控制/不可打印字符替换为空格或 '?'，避免报告里出现乱码。
func cleanPrintable(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\t', '\n', '\r':
			b.WriteRune(' ')
			continue
		}
		if !unicode.IsPrint(r) {
			b.WriteRune('?')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
