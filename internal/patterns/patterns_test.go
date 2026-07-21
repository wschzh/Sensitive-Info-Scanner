package patterns

import (
	"testing"

	"sensitivescanner/internal/types"
)

// TestExamplesMatch 每条模式的官方示例必须能命中。
func TestExamplesMatch(t *testing.T) {
	for _, p := range All() {
		for _, ex := range p.Examples {
			if !p.RE().MatchString(ex) {
				t.Errorf("模式 %q 未匹配其示例 %q\n  pattern=%s", p.Name, ex, p.Pattern)
			}
		}
	}
}

func TestCount(t *testing.T) {
	if got := len(All()); got != 13 {
		t.Errorf("期望 13 条模式，实际 %d", got)
	}
}

func TestIDCardChineseBoundary(t *testing.T) {
	p := FindByName("身份证号")
	if p == nil {
		t.Fatal("未找到身份证模式")
	}
	cases := []string{
		"身份证号：11010519900307234X",
		"身份证 440308199001011234 登记",
		"id=11010519900307234X",
	}
	for _, c := range cases {
		if !p.RE().MatchString(c) {
			t.Errorf("身份证未匹配 %q", c)
		}
	}
}

func TestPhoneBoundary(t *testing.T) {
	p := FindByName("手机号码")
	if p == nil {
		t.Fatal("未找到手机号模式")
	}
	if !p.RE().MatchString("联系电话13812345678速回") {
		t.Error("手机号未在中文边界匹配")
	}
}

func TestWindowsPathLazy(t *testing.T) {
	p := FindByName("内部服务器路径")
	if p == nil {
		t.Fatal("未找到 Windows 路径模式")
	}
	m := p.RE().FindString(`前缀 C:\Users\Admin\Documents 后缀`)
	if m != `C:\Users\Admin\Documents` {
		t.Errorf("Windows 路径惰性匹配失败，got=%q", m)
	}
}

// TestWindowsPathRejectsSingleSegment 收紧后：单层裸串（C:\temp / C:\logs）不应匹配，
// 以减少代码、日志里的字面量误报；真实内部路径几乎都含至少两层目录。
// 注意：两层及以上的字面量（如 C:\logs\app）仍会匹配——正则无法区分语义，
// 进一步治理需上下文校验或误报样本，见 TODO 2.8。
func TestWindowsPathRejectsSingleSegment(t *testing.T) {
	p := FindByName("内部服务器路径")
	if p == nil {
		t.Fatal("未找到 Windows 路径模式")
	}
	for _, c := range []string{`C:\temp`, `路径 C:\logs 结尾`, `D:\`, `盘=C:x`} {
		if p.RE().MatchString(c) {
			t.Errorf("单层路径不应匹配，却命中 %q (pattern=%s)", c, p.Pattern)
		}
	}
}

func TestByLevel(t *testing.T) {
	if got := len(ByLevel(types.Critical)); got != 3 {
		t.Errorf("Critical 期望 3 条，实际 %d", got)
	}
}

func TestExcludedDirCaseInsensitive(t *testing.T) {
	for _, name := range []string{"LOGS", "Temp", "NODE_MODULES", "Dist", "Windows", "PROGRAMDATA", "Program Files (x86)"} {
		if !IsExcludedDir(name) {
			t.Errorf("目录 %q 应大小写不敏感地被排除", name)
		}
	}
}

func TestExcludedNoiseExtensions(t *testing.T) {
	for _, ext := range []string{".out", ".err", ".pid", ".dat", ".cache", ".bin", ".dll", ".exe", ".msi"} {
		if !IsExcludedExt(ext) {
			t.Errorf("扩展名 %q 应被排除", ext)
		}
	}
	if IsExcludedExt(".bak") {
		t.Error(".bak 备份文件可能泄露敏感信息，不应默认排除")
	}
}

func TestExcludedCompoundLogName(t *testing.T) {
	for _, name := range []string{"app.log.1", "access.LOG.20260721"} {
		if !IsExcludedFileName(name) {
			t.Errorf("复合日志文件 %q 应被排除", name)
		}
	}
}
