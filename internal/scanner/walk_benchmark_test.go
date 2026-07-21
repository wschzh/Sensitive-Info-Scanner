package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"sensitivescanner/internal/types"
)

func BenchmarkWalkEngineStd(b *testing.B) {
	benchmarkWalkEngine(b, WalkEngineStd)
}

func BenchmarkWalkEngineFastwalk(b *testing.B) {
	benchmarkWalkEngine(b, WalkEngineFastwalk)
}

func benchmarkWalkEngine(b *testing.B, engine string) {
	dir := buildBenchTree(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := New(Config{WalkEngine: engine, ScanLevels: []types.Level{types.Critical}})
		s.ScanDirectory(dir, true)
	}
}

func buildBenchTree(tb testing.TB) string {
	tb.Helper()
	dir := tb.TempDir()
	for d := 0; d < 25; d++ {
		sub := filepath.Join(dir, fmt.Sprintf("project%02d", d), "src")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			tb.Fatal(err)
		}
		for f := 0; f < 20; f++ {
			path := filepath.Join(sub, fmt.Sprintf("file%02d.txt", f))
			if err := os.WriteFile(path, []byte("ordinary text\n"), 0o644); err != nil {
				tb.Fatal(err)
			}
		}
	}
	for _, skip := range []string{"logs", "cache", "node_modules"} {
		sub := filepath.Join(dir, skip)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			tb.Fatal(err)
		}
		for f := 0; f < 50; f++ {
			path := filepath.Join(sub, fmt.Sprintf("skip%02d.txt", f))
			if err := os.WriteFile(path, []byte("api_key: should_skip_1234567890\n"), 0o644); err != nil {
				tb.Fatal(err)
			}
		}
	}
	return dir
}
