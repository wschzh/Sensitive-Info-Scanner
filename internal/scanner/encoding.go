package scanner

import (
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// decodeText 多编码解码链：UTF-8 优先，失败依次尝试 GBK、GB18030，最后 Latin-1 兜底。
// 对齐原 Python scanner 的 ['utf-8','gbk','gb2312','latin-1'] 尝试顺序（GB18030 是 GBK/GB2312 超集）。
func decodeText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if utf8.Valid(raw) {
		return string(raw)
	}
	if s, err := simplifiedchinese.GBK.NewDecoder().Bytes(raw); err == nil {
		return string(s)
	}
	if s, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw); err == nil {
		return string(s)
	}
	// Latin-1 永不失败：每个字节映射到 U+00xx，保证二进制文件也不报错（只是乱码，不会误报）
	if s, err := charmap.ISO8859_1.NewDecoder().Bytes(raw); err == nil {
		return string(s)
	}
	return string(raw)
}
