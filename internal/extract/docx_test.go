package extract

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDOCXRoundTrip 手工构造最小 docx（zip 含 word/document.xml），验证提取。
func TestDOCXRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.docx")

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>联系电话 13812345678</w:t></w:r></w:p>
    <w:p><w:r><w:t>身份证 11010519900307234X</w:t><w:tab/><w:t>邮箱 a@b.com</w:t></w:r></w:p>
  </w:body>
</w:document>`

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := DOCX(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"13812345678", "11010519900307234X", "a@b.com"} {
		if !strings.Contains(got, want) {
			t.Errorf("docx 提取缺失 %q\ngot=%s", want, got)
		}
	}
}
