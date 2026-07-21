package scanner

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"sensitivescanner/internal/types"
)

// writeFiles 在 dir 下创建 n 个 f0000..txt 文件，内容由 fn(i) 生成。
func writeFiles(t *testing.T, dir string, n int, fn func(i int) string) {
	t.Helper()
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("f%04d.txt", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(fn(i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// 并发扫描正确性：多 worker 下命中数与预期精确一致（不丢/不重）。
func TestConcurrentScanCorrectness(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, 100, func(i int) string {
		return fmt.Sprintf("tel: 139%08d mail: user%02d@example.com\n", i, i)
	})
	s := New(Config{Workers: 8})
	s.ScanDirectory(dir, false)

	stats := s.Stats()
	if stats.ScannedFiles != 100 {
		t.Errorf("ScannedFiles=%d want 100", stats.ScannedFiles)
	}
	// 每文件 1 手机(medium) + 1 邮箱(medium) = 2 命中
	if stats.TotalIssues != 200 {
		t.Errorf("TotalIssues=%d want 200", stats.TotalIssues)
	}
	if stats.IssuesByLevel[types.Medium] != 200 {
		t.Errorf("IssuesByLevel[medium]=%d want 200", stats.IssuesByLevel[types.Medium])
	}
	if got := len(s.Results()); got != 200 {
		t.Errorf("results len=%d want 200", got)
	}
	if stats.TruncatedCount != 0 {
		t.Errorf("TruncatedCount=%d want 0（未达上限）", stats.TruncatedCount)
	}
}

// 结果上限 B'：critical 全留、low 超限截断，但 TotalIssues/IssuesByLevel 始终准确。
func TestResultLimitPriorityRetention(t *testing.T) {
	dir := t.TempDir()
	// 50 文件，每个含 1 low(URL) + 1 critical(api_key)
	writeFiles(t, dir, 50, func(i int) string {
		return fmt.Sprintf("url: https://example.com/p/%d\napi_key: ak_live_%016d\n", i, i)
	})
	// MaxResults=10，KeepLevel 默认 Medium → critical(优先级1) 永远保留，low(优先级4) 超限截断
	s := New(Config{Workers: 4, MaxResults: 10})
	s.ScanDirectory(dir, false)

	stats := s.Stats()
	if stats.TotalIssues != 100 { // 50 URL + 50 api_key
		t.Errorf("TotalIssues=%d want 100", stats.TotalIssues)
	}
	if stats.IssuesByLevel[types.Critical] != 50 {
		t.Errorf("critical count=%d want 50", stats.IssuesByLevel[types.Critical])
	}
	if stats.IssuesByLevel[types.Low] != 50 {
		t.Errorf("low count=%d want 50", stats.IssuesByLevel[types.Low])
	}
	// critical 必须全部保留（优先级保留）
	criticalKept := 0
	for _, r := range s.Results() {
		if r.Level == types.Critical {
			criticalKept++
		}
	}
	if criticalKept != 50 {
		t.Errorf("critical retained=%d want 50（优先级保留应全留）", criticalKept)
	}
	// low 被截断
	if stats.TruncatedByLevel[types.Low] == 0 {
		t.Errorf("期望 low 被截断，TruncatedByLevel[low]=0")
	}
	if stats.TruncatedCount == 0 {
		t.Errorf("期望 TruncatedCount>0")
	}
}

// 分页 ResultsPage 与增量 ResultsSince。
func TestResultsPageAndSince(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, 30, func(i int) string {
		return fmt.Sprintf("tel: 139%08d\n", i) // 每文件 1 手机(medium)
	})
	s := New(Config{Workers: 4})
	s.ScanDirectory(dir, false)

	// 第一页
	page, total := s.ResultsPage(0, 10, ResultFilter{})
	if total != 30 {
		t.Errorf("total=%d want 30", total)
	}
	if len(page) != 10 {
		t.Errorf("page1 len=%d want 10", len(page))
	}
	// 末页（不足一页）
	if last, _ := s.ResultsPage(25, 10, ResultFilter{}); len(last) != 5 {
		t.Errorf("last page len=%d want 5", len(last))
	}
	// offset 超出范围
	if over, _ := s.ResultsPage(40, 10, ResultFilter{}); len(over) != 0 {
		t.Errorf("over page len=%d want 0", len(over))
	}
	// 级别筛选：全 medium，筛 low 为 0、筛 medium 为 30
	if _, n := s.ResultsPage(0, 10, ResultFilter{Level: types.Low}); n != 0 {
		t.Errorf("low total=%d want 0", n)
	}
	if _, n := s.ResultsPage(0, 10, ResultFilter{Level: types.Medium}); n != 30 {
		t.Errorf("medium total=%d want 30", n)
	}
	// 关键字筛选
	if _, n := s.ResultsPage(0, 10, ResultFilter{Keyword: "13900000000"}); n != 1 {
		t.Errorf("keyword total=%d want 1", n)
	}
	// since：fromSeq=25 返回后 5 条
	items, nextSeq := s.ResultsSince(25)
	if len(items) != 5 {
		t.Errorf("since items=%d want 5", len(items))
	}
	if nextSeq != 30 {
		t.Errorf("nextSeq=%d want 30", nextSeq)
	}
}

func TestRemoveResultsForPaths(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	del := filepath.Join(dir, "delete.txt")
	if err := os.WriteFile(keep, []byte("tel: 13900000001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(del, []byte("tel: 13900000002\nmail: user@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(Config{})
	s.ScanDirectory(dir, false)
	if got := s.Stats().TotalIssues; got != 3 {
		t.Fatalf("TotalIssues=%d want 3", got)
	}

	removed := s.RemoveResultsForPaths(map[string]bool{del: true})
	if removed != 2 {
		t.Fatalf("removed=%d want 2", removed)
	}
	stats := s.Stats()
	if stats.TotalIssues != 1 {
		t.Errorf("TotalIssues=%d want 1", stats.TotalIssues)
	}
	if stats.FilesWithIssues != 1 {
		t.Errorf("FilesWithIssues=%d want 1", stats.FilesWithIssues)
	}
	for _, r := range s.Results() {
		if r.FilePath == del {
			t.Fatalf("已删除路径 %s 不应继续出现在结果集中", del)
		}
	}
}

// Stop 后 ScanDirectory 必须返回（不阻塞/不泄漏）。
func TestStopReturns(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, 3000, func(i int) string {
		return string(bytes.Repeat([]byte("filler "), 50)) // 350B/文件，无敏感
	})
	s := New(Config{Workers: 2})
	done := make(chan struct{})
	go func() { s.ScanDirectory(dir, false); close(done) }()
	time.Sleep(20 * time.Millisecond)
	s.Stop()
	select {
	case <-done:
		// good
	case <-time.After(15 * time.Second):
		t.Fatal("ScanDirectory 在 Stop 后未返回（疑似死锁/goroutine 泄漏）")
	}
}

// 跨行模式（私钥）匹配：BEGIN...END 跨多行仍命中，行号取匹配起点所在行。
func TestMatchCrossLinePrivateKey(t *testing.T) {
	dir := t.TempDir()
	content := "header line\n-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEAxxxx\n-----END RSA PRIVATE KEY-----\ntail line\n"
	path := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(Config{})
	results := s.ScanFile(path)
	var key *types.ScanResult
	for i := range results {
		if results[i].PatternName == "私钥" {
			key = &results[i]
			break
		}
	}
	if key == nil {
		t.Fatal("未命中跨行私钥")
	}
	if key.Level != types.Critical {
		t.Errorf("私钥级别=%s want critical", key.Level)
	}
	if key.LineNumber != 2 { // header line 是第 1 行
		t.Errorf("私钥行号=%d want 2", key.LineNumber)
	}
}

// 压测：海量命中（10万+）下 MaxResults 截断 + 统计准确 + 不 OOM。
func TestStressManyResults(t *testing.T) {
	const files = 200
	const urlsPerFile = 500 // 每文件 500 URL(low) + 1 api_key(critical) = 100200 命中
	dir := t.TempDir()
	writeFiles(t, dir, files, func(i int) string {
		var b strings.Builder
		fmt.Fprintf(&b, "api_key: ak_live_%016d\n", i)
		for j := 0; j < urlsPerFile; j++ {
			fmt.Fprintf(&b, "https://example.com/p/%d/%d\n", i, j)
		}
		return b.String()
	})
	s := New(Config{Workers: 8, MaxResults: 10000}) // 小上限强制截断 low
	s.ScanDirectory(dir, false)

	stats := s.Stats()
	wantTotal := files * (urlsPerFile + 1) // 100200
	if stats.TotalIssues != wantTotal {
		t.Errorf("TotalIssues=%d want %d", stats.TotalIssues, wantTotal)
	}
	criticalKept := 0
	for _, r := range s.Results() {
		if r.Level == types.Critical {
			criticalKept++
		}
	}
	if criticalKept != files {
		t.Errorf("critical retained=%d want %d（优先级保留应全留）", criticalKept, files)
	}
	if stats.TruncatedByLevel[types.Low] == 0 {
		t.Errorf("期望 low 被大量截断，实际 TruncatedByLevel[low]=0")
	}
}

// 非常规文件（FIFO/管道）应被 readFile 跳过，避免 os.ReadFile 永久阻塞拖垮 worker。
func TestSkipNonRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows 无 mkfifo，跳过")
	}
	dir := t.TempDir()
	pipe := filepath.Join(dir, "named.pipe")
	if err := syscall.Mkfifo(pipe, 0666); err != nil {
		t.Skipf("无法创建 FIFO: %v", err)
	}
	s := New(Config{})
	if _, ok := s.readFile(pipe); ok {
		t.Error("FIFO（非常规文件）应被 readFile 跳过，否则 ReadFile 会永久阻塞")
	}
}

// 单行巨内容（如压缩 JS 一行）：matchContent 的 LineContent 必须截断到匹配附近，
// 否则每条结果都存整行会导致内存爆炸 + 极慢。（直接测 matchContent，绕过 ScanFile 的 isMinified 跳过）
func TestMinifiedSingleLineNotExplode(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 1000; i++ { // 单行 1000 个 URL
		fmt.Fprintf(&b, "var u%d='https://example.com/p/%d';", i, i)
	}
	s := New(Config{})
	results := s.matchContent("big.min.js", b.String())
	if len(results) == 0 {
		t.Fatal("未命中 URL")
	}
	wholeLine := b.Len()
	for _, r := range results {
		if len(r.LineContent) > 2*(lineContextPad)+200 {
			t.Errorf("LineContent 未截断，长度 %d（整行 %d），会导致内存爆炸", len(r.LineContent), wholeLine)
		}
	}
}

// minified 压缩文件（单行超 4KB）应被 ScanFile 跳过：其中的 URL/IP/数字多为字面量噪音。
// 同时验证正常多行文件（含真实密钥）仍被正常扫描。
func TestSkipMinified(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < 1000; i++ { // 单行 > 4KB
		fmt.Fprintf(&b, "var u%d='https://x.example.com/%d';", i, i)
	}
	minPath := filepath.Join(dir, "a.min.js")
	if err := os.WriteFile(minPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	normalPath := filepath.Join(dir, "config.txt")
	if err := os.WriteFile(normalPath, []byte("password: secret123\nusername: admin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(Config{})
	if r := s.ScanFile(minPath); len(r) != 0 {
		t.Errorf("minified 文件应被跳过，实际返回 %d 条", len(r))
	}
	if r := s.ScanFile(normalPath); len(r) == 0 {
		t.Error("正常多行文件不应被跳过")
	}
}

// 大多行文件（如大 XML）：全文一次性匹配应快速完成，不卡
// （逐行版会因「行数 × 13 pattern」次调用而极慢）。
func TestLargeMultiLineFileFast(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100000; i++ { // 10 万行，每行 1 手机号
		fmt.Fprintf(&b, "line %d tel: 139%08d end\n", i, i)
	}
	s := New(Config{})
	results := s.matchContent("big.xml", b.String())
	if len(results) == 0 {
		t.Fatal("未命中手机号")
	}
	if len(results) > maxMatchesPerLine {
		t.Errorf("单 pattern 匹配数 %d 超过上限 %d", len(results), maxMatchesPerLine)
	}
}
