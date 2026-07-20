package extract

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestXLSXRoundTrip 用 excelize 生成含敏感信息的 xlsx，再提取验证。
func TestXLSXRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.xlsx")

	f := excelize.NewFile()
	defer f.Close()
	rows := [][]any{
		{"手机号", "13812345678"},
		{"身份证", "11010519900307234X"},
		{"邮箱", "a@b.com"},
		{"备注", "正常内容"},
	}
	for i, r := range rows {
		if err := f.SetSheetRow("Sheet1", "A"+itoa(i+1), &r); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SaveAs(p); err != nil {
		t.Fatal(err)
	}

	got, err := XLSX(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"13812345678", "11010519900307234X", "a@b.com"} {
		if !strings.Contains(got, want) {
			t.Errorf("xlsx 提取缺失 %q\ngot=%s", want, got)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
