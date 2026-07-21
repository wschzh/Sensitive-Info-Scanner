// Package extract 从特殊格式文件（xlsx/docx/pdf/图片）提取纯文本，供 scanner 做正则匹配。
package extract

import (
	"archive/zip"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// maxXLSXUncompressed xlsx 解压后大小上限（字节）。
// xlsx 是 zip 压缩，压缩比可达 10-50x，scanner 层的 MaxFileSize 只拦压缩前大小、对它几乎无效；
// 此处在 extract 层做二次保护：解压后超过该阈值则跳过，防止大表把内存撑爆。
const maxXLSXUncompressed = 50 * 1024 * 1024 // 50MB

// XLSX 提取 xlsx 文件所有工作表的文本（单元格以 \t 分隔，对齐原 Python 实现）。
// 用 f.Rows 流式逐行迭代替代 GetRows 全量物化（避免 [][]string 双重驻留），降低大表内存峰值。
func XLSX(path string) (string, error) {
	if size, err := uncompressedSize(path); err == nil && size > maxXLSXUncompressed {
		// 解压后过大：直接放弃，避免后续 OpenFile + 拼接把进程拖垮（与其它 extract 失败一致，静默跳过）。
		return "", fmt.Errorf("xlsx 解压后约 %dMB 超过 %dMB 上限，跳过以防 OOM",
			size/1024/1024, maxXLSXUncompressed/1024/1024)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var b strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.Rows(sheet)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "=== 工作表: %s ===\n", sheet)
		for rows.Next() {
			row, err := rows.Columns()
			if err != nil {
				continue
			}
			if !rowHasContent(row) {
				continue
			}
			b.WriteString(strings.Join(row, "\t"))
			b.WriteByte('\n')
		}
		_ = rows.Close() // 释放迭代器持有的流式 reader
	}
	return b.String(), nil
}

// uncompressedSize 累加 xlsx(zip) 内所有条目的解压后大小，用于 OOM 预估。
// 非 zip 或读取失败时返回 err，调用方据此放行（交给 excelize 自行报错）。
func uncompressedSize(path string) (int64, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	var total int64
	for _, file := range zr.File {
		total += int64(file.UncompressedSize64)
	}
	return total, nil
}

func rowHasContent(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return true
		}
	}
	return false
}
