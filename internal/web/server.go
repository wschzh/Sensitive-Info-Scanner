//go:build gui

// Package web 提供 Web 图形界面：内嵌单页前端 + REST API，监听 127.0.0.1 随机端口。
package web

import (
	_ "embed"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"sync"

	"sensitivescanner/internal/report"
	"sensitivescanner/internal/scanner"
	"sensitivescanner/internal/types"
)

//go:embed index.html
var indexHTML []byte

// Server Web 扫描服务。
type Server struct {
	mu       sync.Mutex
	scanner  *scanner.Scanner
	scanning bool
}

// NewServer 创建服务（初始扫描器为默认配置）。
func NewServer() *Server {
	return &Server{scanner: scanner.New(scanner.Config{})}
}

// Start 在 127.0.0.1 随机端口启动 HTTP 服务，返回监听地址。
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := ln.Addr().String()
	go func() { _ = http.Serve(ln, s.routes()) }()
	return addr, nil
}

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/scan", s.handleScan)
	mux.HandleFunc("/api/progress", s.handleProgress)
	mux.HandleFunc("/api/results", s.handleResults)
	mux.HandleFunc("/api/stop", s.handleStop)
	mux.HandleFunc("/api/report", s.handleReport)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

type scanRequest struct {
	Path      string        `json:"path"`
	Recursive bool          `json:"recursive"`
	MaxSize   int64         `json:"max_size"`
	Levels    []types.Level `json:"levels"`
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST", http.StatusMethodNotAllowed)
		return
	}
	var req scanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path 不能为空", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(req.Path); err != nil {
		http.Error(w, "路径不存在: "+req.Path, http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.scanning {
		s.mu.Unlock()
		http.Error(w, "已有扫描在进行", http.StatusConflict)
		return
	}
	s.scanner = scanner.New(scanner.Config{MaxFileSize: req.MaxSize, ScanLevels: req.Levels})
	s.scanning = true
	s.mu.Unlock()

	go func() {
		if fi, _ := os.Stat(req.Path); fi.IsDir() {
			s.scanner.ScanDirectory(req.Path, req.Recursive)
		} else {
			s.scanner.ScanSingle(req.Path)
		}
		s.mu.Lock()
		s.scanning = false
		s.mu.Unlock()
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	scanning := s.scanning
	s.mu.Unlock()
	progress, current := s.scanner.Progress()
	stats := s.scanner.Stats()
	writeJSON(w, map[string]any{
		"progress":     progress,
		"current_file": current,
		"scanning":     scanning,
		"stats":        stats,
	})
}

func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"results": s.scanner.Results(),
		"stats":   s.scanner.Stats(),
	})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.scanner.Stop()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	format := report.Text
	if r.URL.Query().Get("format") == "json" {
		format = report.JSON
	}
	content, _ := report.Generate(s.scanner, format, "")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
