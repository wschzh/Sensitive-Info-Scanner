// Package patterns 定义敏感信息正则模式、文件分类与排除规则。
// 正则从原 Python sensitive_patterns.py 迁移并适配 Go RE2 语法：
//   - 每条 Pattern 前加 (?i) 实现忽略大小写
//   - 身份证去掉前导 [^\d]?（紧贴中文会吞字符）
//   - 手机号加前 \b（防止从长数字串中间截取）
//   - 数据库连接串的 data source / user id 允许空格或下划线分隔（适配 "Data Source" 真实写法）
//   - Windows 路径用贪婪量词并排除空格（在空格/引号处自然停止，避免吃到行尾）
package patterns

import (
	"regexp"
	"sensitivescanner/internal/types"
)

// Pattern 单条敏感信息模式。
type Pattern struct {
	Name        string
	Pattern     string
	Level       types.Level
	Description string
	Examples    []string
	CrossLine   bool // 是否需要跨行匹配（如私钥 -----BEGIN...END-----）
	re          *regexp.Regexp
}

// RE 返回已编译的正则。
func (p *Pattern) RE() *regexp.Regexp { return p.re }

// all 全部模式，启动时统一编译。
var all = compileAll([]Pattern{
	// ---------------- Critical 严重 ----------------
	{
		Name:        "私钥",
		Level:       types.Critical,
		Pattern:     `(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----[\s\S]+?-----END\s+(RSA\s+)?PRIVATE\s+KEY-----`,
		Description: "RSA私钥或其他加密私钥",
		CrossLine:   true,
		Examples: []string{
			"-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEAbGcexample\n-----END RSA PRIVATE KEY-----",
		},
	},
	{
		Name:        "API密钥",
		Level:       types.Critical,
		Pattern:     `(?i)(?:api[_-]?key|apikey|api[_-]?secret|access[_-]?token|auth[_-]?token|secret[_-]?key|private[_-]?key)[\s:=]+["']?[a-zA-Z0-9_\-]{16,}["']?`,
		Description: "各种API密钥和访问令牌",
		Examples:    []string{"api_key: abc123def456ghi789"},
	},
	{
		Name:        "数据库连接字符串",
		Level:       types.Critical,
		Pattern:     `(?i)(?:data[\s_-]*source|server|host)[\s=]+["']?[^\s"';]+["']?[\s;]*(?:user[\s_-]*id|uid|user)[\s=]+["']?[^\s"';]+["']?[\s;]*(?:password|pwd)[\s=]+["']?[^\s"';]+["']?`,
		Description: "包含用户名和密码的数据库连接字符串",
		Examples:    []string{"Data Source=localhost;User ID=admin;Password=secret123"},
	},

	// ---------------- High 高级 ----------------
	{
		Name:        "身份证号",
		Level:       types.High,
		Pattern:     `(?i)\b[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`,
		Description: "中国身份证号码",
		Examples:    []string{"身份证号：11010519900307234X", "440308199001011234"},
	},
	{
		Name:        "银行卡号",
		Level:       types.High,
		Pattern:     `\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3[0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\b`,
		Description: "主流银行卡号",
		Examples:    []string{"4111111111111111"},
	},
	{
		Name:        "密码",
		Level:       types.High,
		Pattern:     `(?i)(?:password|passwd|pwd|pass)\s*[:=]\s*["']?[^\s"';]{6,}["']?`,
		Description: "密码字段",
		Examples:    []string{"password: secret123", `pwd="mypassword"`},
	},
	{
		Name:        "JWT令牌",
		Level:       types.High,
		Pattern:     `eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`,
		Description: "JWT认证令牌",
		Examples:    []string{"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.SflKxwRJSmeKKF2QT4fwpMeJf36POk6yJVadQssw5c"},
	},

	// ---------------- Medium 中级 ----------------
	{
		Name:        "手机号码",
		Level:       types.Medium,
		Pattern:     `\b1[3-9]\d{9}\b`,
		Description: "中国大陆手机号码",
		Examples:    []string{"13812345678", "手机号：15987654321"},
	},
	{
		Name:        "邮箱地址",
		Level:       types.Medium,
		Pattern:     `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`,
		Description: "电子邮件地址",
		Examples:    []string{"user@example.com"},
	},
	{
		Name:        "IP地址",
		Level:       types.Low,
		Pattern:     `\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`,
		Description: "IPv4地址（日志/代码中多为误报，已降为低级）",
		Examples:    []string{"192.168.1.1"},
	},

	// ---------------- Low 低级 ----------------
	{
		Name:        "用户名",
		Level:       types.Low,
		Pattern:     `(?i)(?:username|user|login|userid)\s*[:=]\s*["']?[a-zA-Z0-9_\-]{3,}["']?`,
		Description: "用户名字段",
		Examples:    []string{"username: admin"},
	},
	{
		Name:        "URL地址",
		Level:       types.Low,
		Pattern:     `(?i)https?://[^\s<"']+`,
		Description: "HTTP/HTTPS URL地址",
		Examples:    []string{"https://www.example.com"},
	},
	{
		Name:        "内部服务器路径",
		Level:       types.Low,
		Pattern:     `[A-Za-z]:\\[^<>:"|?*\r\n ]+`,
		Description: "Windows文件路径（遇空格/引号停止）",
		Examples:    []string{`C:\Users\Admin\Documents`},
	},
})

func compileAll(ps []Pattern) []Pattern {
	out := make([]Pattern, len(ps))
	for i, p := range ps {
		p.re = regexp.MustCompile(p.Pattern)
		out[i] = p
	}
	return out
}

// All 返回全部已编译模式。
func All() []Pattern { return all }

// ByLevel 返回指定级别集合内的模式。
func ByLevel(levels ...types.Level) []Pattern {
	want := make(map[types.Level]bool, len(levels))
	for _, l := range levels {
		want[l] = true
	}
	var out []Pattern
	for _, p := range all {
		if want[p.Level] {
			out = append(out, p)
		}
	}
	return out
}

// FindByName 按名称查找（测试用）。
func FindByName(name string) *Pattern {
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}

// SplitCrossLine 把模式集拆为单行与跨行两组，供扫描器分流匹配（跨行整段、单行逐行）。
func SplitCrossLine(ps []Pattern) (single, cross []Pattern) {
	for _, p := range ps {
		if p.CrossLine {
			cross = append(cross, p)
		} else {
			single = append(single, p)
		}
	}
	return single, cross
}

// FileCategories 文件扩展名分类（迁移自原 FILE_CATEGORIES，去重）。
var FileCategories = map[string][]string{
	"code":     {".py", ".js", ".java", ".c", ".cpp", ".cs", ".php", ".rb", ".go", ".rs", ".swift", ".kt", ".scala", ".sh", ".bat", ".ps1"},
	"config":   {".json", ".xml", ".yml", ".yaml", ".ini", ".conf", ".cfg", ".config", ".properties", ".toml"},
	"document": {".txt", ".md", ".doc", ".docx", ".pdf", ".rtf", ".odt"},
	"web":      {".html", ".htm", ".css", ".scss", ".less", ".vue", ".jsx", ".tsx"},
	"data":     {".sql", ".db", ".sqlite", ".csv", ".xls", ".xlsx", ".accdb"},
	"image":    {".jpg", ".jpeg", ".png", ".bmp", ".tiff", ".gif", ".webp"},
}

// scannableExt 所有可扫描扩展名集合。
var scannableExt = func() map[string]bool {
	m := map[string]bool{}
	for _, exts := range FileCategories {
		for _, e := range exts {
			m[e] = true
		}
	}
	return m
}()

// IsScannableExt 扩展名是否可扫描。
func IsScannableExt(ext string) bool { return scannableExt[ext] }

// ExcludeDirs 需排除的目录名（walker 按路径分段精确匹配，避免误伤 "binary" 等）。
var ExcludeDirs = []string{
	".git", ".svn", ".hg", "node_modules", "__pycache__",
	".vscode", ".idea", "bin", "obj", "dist", "build", "temp", "tmp",
	"System Volume Information", "$Recycle.Bin",
}

// excludeDirSet 排除目录名集合。
var excludeDirSet = func() map[string]bool {
	m := map[string]bool{}
	for _, d := range ExcludeDirs {
		m[d] = true
	}
	return m
}()

// IsExcludedDir 目录名是否在排除集合。
func IsExcludedDir(name string) bool { return excludeDirSet[name] }

// ExcludeExtensions 需排除的扩展名（原 .log$/.tmp$）。
var ExcludeExtensions = []string{".log", ".tmp"}

// IsExcludedExt 扩展名是否在排除集合。
func IsExcludedExt(ext string) bool {
	for _, e := range ExcludeExtensions {
		if e == ext {
			return true
		}
	}
	return false
}
