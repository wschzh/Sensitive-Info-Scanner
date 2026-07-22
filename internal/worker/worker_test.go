package worker

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sensitivescanner/internal/types"
	"sensitivescanner/internal/workerproto"
)

func TestRunExtractWorkerDOCX(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.docx")
	writeMinimalDOCX(t, path, "api_key: abcdefghijklmnop1234")

	req := workerproto.Request{
		Path:   path,
		Format: "docx",
		Levels: []types.Level{types.Critical, types.High},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := RunExtractWorker(bytes.NewReader(body), &out)
	if code != 0 {
		t.Fatalf("worker exit=%d out=%s", code, out.String())
	}
	var resp workerproto.Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" {
		t.Fatalf("status=%s error=%s", resp.Status, resp.Error)
	}
	if len(resp.Findings) != 1 {
		t.Fatalf("findings=%d want 1: %s", len(resp.Findings), out.String())
	}
	if resp.Findings[0].PatternName != "API密钥" {
		t.Fatalf("pattern=%s want API密钥", resp.Findings[0].PatternName)
	}
}

func writeMinimalDOCX(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` +
		escapeXML(text) +
		`</w:t></w:r></w:p></w:body></w:document>`
	if _, err := w.Write([]byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
