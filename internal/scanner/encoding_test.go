package scanner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// utf8BOM 用字节字面量表示（源码中禁止出现裸 BOM 字符，否则编译报 illegal byte order mark）。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func withBOM(s string) []byte { return append(append([]byte{}, utf8BOM...), s...) }

// 验证 decodeText 剥离 UTF-8 BOM（修复前 BOM 会残留在正文头部）。
func TestDecodeTextStripsUTF8BOM(t *testing.T) {
	got := decodeText(withBOM("password: secret123"))
	if bytes.HasPrefix([]byte(got), utf8BOM) {
		t.Fatalf("解码后仍含 BOM: %q", got)
	}
	if got != "password: secret123" {
		t.Fatalf("解码结果不符: %q", got)
	}
}

// 验证带 UTF-8 BOM 的文件首行命中：行号正确为 1，行内容不含 BOM。
func TestScanFileBOMFirstLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	// 首行带 BOM + 敏感信息（手机号），第二行普通文本
	content := withBOM("contact: 13812345678\nsecond line\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(Config{})
	results := s.ScanFile(path)
	if len(results) == 0 {
		t.Fatal("未命中手机号")
	}
	r := results[0]
	if r.LineNumber != 1 {
		t.Errorf("首行命中行号应为 1，实际 %d", r.LineNumber)
	}
	if bytes.Contains([]byte(r.LineContent), utf8BOM) {
		t.Errorf("行内容含 BOM: %q", r.LineContent)
	}
	if !strings.Contains(r.LineContent, "13812345678") {
		t.Errorf("行内容缺失匹配项: %q", r.LineContent)
	}
}
