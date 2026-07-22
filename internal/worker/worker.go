package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"sensitivescanner/internal/extract"
	"sensitivescanner/internal/scanner"
	"sensitivescanner/internal/workerproto"
)

// RunExtractWorker runs the hidden one-shot rich-document worker.
func RunExtractWorker(stdin io.Reader, stdout io.Writer) int {
	start := time.Now()
	resp := workerproto.Response{Status: "error"}
	defer func() {
		resp.DurationMillis = time.Since(start).Milliseconds()
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(resp)
	}()
	defer func() {
		if v := recover(); v != nil {
			resp.Status = "panic"
			resp.Error = fmt.Sprintf("%v", v)
			fmt.Fprintf(os.Stderr, "extract worker panic: %v\n%s", v, debug.Stack())
		}
	}()

	var req workerproto.Request
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		resp.Error = "decode request: " + err.Error()
		return 2
	}
	if req.MemoryLimitMB > 0 {
		debug.SetMemoryLimit(int64(req.MemoryLimitMB) * 1024 * 1024)
	}

	content, err := extractRichText(req.Path, req.Format)
	if err != nil {
		resp.Error = err.Error()
		return 3
	}
	if req.MaxTextSize > 0 && len(content) > req.MaxTextSize {
		resp.Error = fmt.Sprintf("extracted text %d bytes exceeds limit %d", len(content), req.MaxTextSize)
		resp.Status = "too_large"
		return 4
	}
	if content == "" {
		resp.Status = "completed"
		return 0
	}

	sc := scanner.New(scanner.Config{
		ScanLevels:  req.Levels,
		ScanProfile: req.Mode,
		MaxTextSize: req.MaxTextSize,
	})
	findings := sc.MatchContent(req.Path, content)
	resp.Status = "completed"
	resp.Findings = findings
	resp.MatchedCount = len(findings)
	for _, f := range findings {
		if f.FileIssueOverflow {
			resp.IssueOverflow = true
			break
		}
	}
	return 0
}

func extractRichText(path, format string) (string, error) {
	ext := strings.ToLower(format)
	if ext == "" {
		ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	}
	switch ext {
	case "xlsx":
		return extract.XLSX(path)
	case "docx":
		return extract.DOCX(path)
	case "pdf":
		return extract.PDF(path)
	default:
		return "", fmt.Errorf("unsupported rich format %q", ext)
	}
}
