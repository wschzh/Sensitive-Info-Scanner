package scanner

import (
	"strings"
	"testing"
)

func BenchmarkMatchContentNoHints(b *testing.B) {
	var content strings.Builder
	for i := 0; i < 2000; i++ {
		content.WriteString("ordinary application text without secret labels or network locators\n")
	}
	s := New(Config{})
	text := content.String()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.matchContent("ordinary.txt", text)
	}
}

func BenchmarkMatchContentNoDigits(b *testing.B) {
	var content strings.Builder
	for i := 0; i < 2000; i++ {
		content.WriteString("plain words only without numeric candidates in this line\n")
	}
	s := New(Config{})
	text := content.String()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.matchContent("words.txt", text)
	}
}

func BenchmarkMatchContentWithHints(b *testing.B) {
	text := strings.Repeat("ordinary filler line\n", 1000) +
		"api_key: abc123def456ghi789\n" +
		strings.Repeat("more ordinary filler line\n", 1000)
	s := New(Config{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.matchContent("secret.txt", text)
	}
}
