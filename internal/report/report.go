// Package report 生成 text/json 扫描报告。
// 写文件时统一加 UTF-8 BOM，确保 Windows 记事本能正确显示中文。
package report

import (
	"encoding/json"
	"fmt"
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

// Generate 按 format 生成报告字符串；output 非空则写入文件（带 BOM）。
func Generate(s *scanner.Scanner, format Format, output string) (string, error) {
	var content string
	switch format {
	case JSON:
		content = genJSON(s)
	default:
		content = genText(s)
	}
	if output != "" {
		if err := writeFileWithBOM(output, content); err != nil {
			return content, err
		}
	}
	return content, nil
}

func writeFileWithBOM(path, content string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	_, err = f.WriteString(content)
	return err
}

func genText(s *scanner.Scanner) string {
	stats := s.Stats()
	results := s.Results()

	var b strings.Builder
	b.WriteString(strings.Repeat("=", 80) + "\n")
	b.WriteString("敏感信息扫描报告\n")
	b.WriteString(strings.Repeat("=", 80) + "\n")
	fmt.Fprintf(&b, "扫描时长: %.2f 秒\n", stats.ScanDuration)
	fmt.Fprintf(&b, "总文件数: %d  已扫描: %d  有问题: %d  问题总数: %d\n\n",
		stats.TotalFiles, stats.ScannedFiles, stats.FilesWithIssues, stats.TotalIssues)

	// 按级别分布
	b.WriteString("问题级别分布:\n")
	for _, lv := range types.AllLevels {
		if c := stats.IssuesByLevel[lv]; c > 0 {
			fmt.Fprintf(&b, "  %s: %d\n", types.LevelConfig[lv].Name, c)
		}
	}
	b.WriteString("\n")

	// 按级别分组展示明细
	byLevel := map[types.Level][]types.ScanResult{}
	for _, r := range results {
		byLevel[r.Level] = append(byLevel[r.Level], r)
	}
	for _, lv := range types.AllLevels {
		rs := byLevel[lv]
		if len(rs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "[%s] %d 条\n", types.LevelConfig[lv].Name, len(rs))
		for _, r := range rs {
			content := cleanPrintable(r.LineContent)
			if len([]rune(content)) > 100 {
				content = string([]rune(content)[:100]) + "..."
			}
			fmt.Fprintf(&b, "  文件: %s:%d\n    类型: %s\n    内容: %s\n    匹配: %s\n",
				r.FilePath, r.LineNumber, r.PatternName, content, cleanPrintable(r.MatchedText))
		}
		b.WriteString("\n")
	}
	if len(results) == 0 {
		b.WriteString("未发现敏感信息\n")
	}
	return b.String()
}

func genJSON(s *scanner.Scanner) string {
	stats := s.Stats()
	data := map[string]any{
		"scan_duration": stats.ScanDuration,
		"statistics": map[string]any{
			"total_files":       stats.TotalFiles,
			"scanned_files":     stats.ScannedFiles,
			"files_with_issues": stats.FilesWithIssues,
			"total_issues":      stats.TotalIssues,
			"issues_by_level":   stats.IssuesByLevel,
		},
		"results": s.Results(),
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b)
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
