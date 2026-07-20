// Package extract 从特殊格式文件（xlsx/docx/pdf/图片）提取纯文本，供 scanner 做正则匹配。
package extract

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// XLSX 提取 xlsx 文件所有工作表的文本（单元格以 \t 分隔，对齐原 Python 实现）。
func XLSX(path string) (string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var b strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "=== 工作表: %s ===\n", sheet)
		for _, row := range rows {
			if !rowHasContent(row) {
				continue
			}
			b.WriteString(strings.Join(row, "\t"))
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

func rowHasContent(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return true
		}
	}
	return false
}
