package extract

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDF 提取 PDF 各页的纯文本（仅文字版 PDF；扫描件需走 OCR，超出本工具范围）。
func PDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var b strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		fmt.Fprintf(&b, "=== PDF 页面 %d ===\n", i)
		if txt, err := page.GetPlainText(nil); err == nil {
			b.WriteString(txt)
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}
