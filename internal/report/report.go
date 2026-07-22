// Package report 流式生成 text/json 扫描报告（写 io.Writer，不累积全量字符串）。
// 写文件时统一加 UTF-8 BOM，确保 Windows 记事本能正确显示中文。
package report

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"

	"sensitivescanner/internal/scanner"
	"sensitivescanner/internal/types"
)

// Format 报告格式。
type Format string

const (
	Text Format = "text"
	JSON Format = "json"
	HTML Format = "html"
	MD   Format = "md"
	XLSX Format = "xlsx"
)

const sep = "================================================================================"

// GenerateTo 按 format 流式生成报告到 w（不累积全量字符串，适合 HTTP 响应/大结果集）。
func GenerateTo(s *scanner.Scanner, format Format, w io.Writer) error {
	switch format {
	case JSON:
		return genJSONTo(s, w)
	case HTML:
		return genHTMLTo(s, w)
	case MD:
		return genMarkdownTo(s, w)
	case XLSX:
		return genXLSXTo(s, w)
	default:
		return genTextTo(s, w)
	}
}

// GenerateToFile 生成报告到文件；text 格式先写 UTF-8 BOM。
func GenerateToFile(s *scanner.Scanner, format Format, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if format == Text || format == MD {
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
		if format == Text || format == MD {
			data = WithBOM(content)
		}
		if err := os.WriteFile(output, data, 0o644); err != nil {
			return content, err
		}
	}
	return content, nil
}

func genMarkdownTo(s *scanner.Scanner, w io.Writer) error {
	stats := s.Stats()
	fmt.Fprintf(w, "# 敏感信息扫描报告\n\n")
	fmt.Fprintf(w, "- 扫描时长: %.2f 秒\n", stats.ScanDuration)
	fmt.Fprintf(w, "- 总文件数: %d\n- 已扫描: %d\n- 有问题文件: %d\n- 问题总数: %d\n",
		stats.TotalFiles, stats.ScannedFiles, stats.FilesWithIssues, stats.TotalIssues)
	if stats.SkippedFiles > 0 || stats.SkippedDirs > 0 {
		fmt.Fprintf(w, "- 已跳过: %d 个文件 / %d 个目录\n", stats.SkippedFiles, stats.SkippedDirs)
	}
	fmt.Fprintf(w, "\n## 问题级别分布\n\n| 级别 | 数量 |\n| --- | ---: |\n")
	for _, lv := range types.AllLevels {
		fmt.Fprintf(w, "| %s | %d |\n", types.LevelConfig[lv].Name, stats.IssuesByLevel[lv])
	}
	fmt.Fprintf(w, "\n## 问题文件\n\n")
	files := s.ResultsByFile(scanner.ResultFilter{})
	if len(files) == 0 {
		fmt.Fprintf(w, "未发现敏感信息。\n")
		return nil
	}
	for _, f := range files {
		count := issueCountText(f.IssueCount, f.IssueOverflow)
		fmt.Fprintf(w, "### [%s] %s\n\n", types.LevelConfig[f.HighestLevel].Name, f.FilePath)
		fmt.Fprintf(w, "- 问题数: %s\n- 类型: %s\n\n", count, strings.Join(f.PatternNames, "、"))
		for _, r := range f.Samples {
			fmt.Fprintf(w, "- 第 %d 行 `%s`: `%s`\n", r.LineNumber, r.PatternName, cleanInline(r.MatchedText))
		}
		fmt.Fprintf(w, "\n")
	}
	return nil
}

func genHTMLTo(s *scanner.Scanner, w io.Writer) error {
	stats := s.Stats()
	files := s.ResultsByFile(scanner.ResultFilter{})
	fmt.Fprintf(w, "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>敏感信息扫描报告</title>")
	fmt.Fprintf(w, "<style>body{font-family:-apple-system,BlinkMacSystemFont,'Microsoft YaHei',sans-serif;margin:24px;color:#1f2933;background:#f6f8fb}main{max-width:1180px;margin:auto}.card{background:#fff;border:1px solid #e3e8ef;border-radius:8px;padding:16px;margin:14px 0}table{width:100%%;border-collapse:collapse;background:#fff}th,td{border-bottom:1px solid #e7edf3;padding:9px;text-align:left;font-size:13px}th{background:#eef3f8}.critical{color:#c0392b;font-weight:700}.high{color:#d35400;font-weight:700}.medium{color:#9a6b00}.low{color:#2878b5}.muted{color:#718096;font-size:12px}</style></head><body><main>")
	fmt.Fprintf(w, "<h1>敏感信息扫描报告</h1><div class=\"card\">")
	fmt.Fprintf(w, "<p>扫描时长: %.2f 秒</p><p>总文件数: %d，已扫描: %d，有问题文件: %d，问题总数: %d，跳过: %d 文件 / %d 目录</p>",
		stats.ScanDuration, stats.TotalFiles, stats.ScannedFiles, stats.FilesWithIssues, stats.TotalIssues, stats.SkippedFiles, stats.SkippedDirs)
	fmt.Fprintf(w, "</div><div class=\"card\"><h2>级别分布</h2><table><tr><th>级别</th><th>数量</th></tr>")
	for _, lv := range types.AllLevels {
		fmt.Fprintf(w, "<tr><td class=\"%s\">%s</td><td>%d</td></tr>", lv, types.LevelConfig[lv].Name, stats.IssuesByLevel[lv])
	}
	fmt.Fprintf(w, "</table></div><div class=\"card\"><h2>问题文件</h2>")
	if len(files) == 0 {
		fmt.Fprintf(w, "<p>未发现敏感信息。</p>")
	} else {
		fmt.Fprintf(w, "<table><tr><th>最高级别</th><th>问题数</th><th>类型</th><th>文件</th><th>示例</th></tr>")
		for _, f := range files {
			var samples []string
			for _, r := range f.Samples {
				samples = append(samples, fmt.Sprintf("第%d行 %s: %s", r.LineNumber, r.PatternName, cleanPrintable(r.MatchedText)))
			}
			fmt.Fprintf(w, "<tr><td class=\"%s\">%s</td><td>%s</td><td>%s</td><td>%s</td><td class=\"muted\">%s</td></tr>",
				f.HighestLevel, types.LevelConfig[f.HighestLevel].Name, issueCountText(f.IssueCount, f.IssueOverflow),
				html.EscapeString(strings.Join(f.PatternNames, "、")), html.EscapeString(f.FilePath), html.EscapeString(strings.Join(samples, "\n")))
		}
		fmt.Fprintf(w, "</table>")
	}
	fmt.Fprintf(w, "</div></main></body></html>")
	return nil
}

func genXLSXTo(s *scanner.Scanner, w io.Writer) error {
	stats := s.Stats()
	f := excelize.NewFile()
	defer f.Close()
	summary := "汇总"
	filesSheet := "问题文件"
	detailSheet := "命中明细"
	f.SetSheetName("Sheet1", summary)
	_, _ = f.NewSheet(filesSheet)
	_, _ = f.NewSheet(detailSheet)

	setRow := func(sheet string, row int, vals ...any) {
		for i, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	setRow(summary, 1, "指标", "数值")
	setRow(summary, 2, "扫描时长(秒)", stats.ScanDuration)
	setRow(summary, 3, "总文件数", stats.TotalFiles)
	setRow(summary, 4, "已扫描文件", stats.ScannedFiles)
	setRow(summary, 5, "有问题文件", stats.FilesWithIssues)
	setRow(summary, 6, "问题总数", stats.TotalIssues)
	setRow(summary, 7, "跳过文件", stats.SkippedFiles)
	setRow(summary, 8, "跳过目录", stats.SkippedDirs)
	row := 10
	setRow(summary, row, "级别", "数量")
	row++
	for _, lv := range types.AllLevels {
		setRow(summary, row, types.LevelConfig[lv].Name, stats.IssuesByLevel[lv])
		row++
	}

	setRow(filesSheet, 1, "最高级别", "问题数", "类型", "文件")
	for i, item := range s.ResultsByFile(scanner.ResultFilter{}) {
		setRow(filesSheet, i+2, types.LevelConfig[item.HighestLevel].Name, issueCountText(item.IssueCount, item.IssueOverflow), strings.Join(item.PatternNames, "、"), item.FilePath)
	}

	setRow(detailSheet, 1, "级别", "类型", "文件", "行号", "匹配内容", "上下文")
	for i, r := range s.Results() {
		setRow(detailSheet, i+2, types.LevelConfig[r.Level].Name, r.PatternName, r.FilePath, r.LineNumber, cleanPrintable(r.MatchedText), cleanPrintable(r.LineContent))
	}
	_ = f.SetColWidth(summary, "A", "B", 18)
	_ = f.SetColWidth(filesSheet, "A", "D", 24)
	_ = f.SetColWidth(detailSheet, "A", "F", 22)
	return f.Write(w)
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

func issueCountText(count int, overflow bool) string {
	if overflow {
		return fmt.Sprintf("%d+", count)
	}
	return fmt.Sprintf("%d", count)
}

func cleanInline(s string) string {
	return strings.ReplaceAll(cleanPrintable(s), "`", "'")
}
