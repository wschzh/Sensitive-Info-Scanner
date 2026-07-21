//go:build gui

// Package web 提供 Web 图形界面：内嵌单页前端 + REST API，监听 127.0.0.1 随机端口。
package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"sensitivescanner/internal/patterns"
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
	done     chan struct{}
}

// NewServer 创建服务（初始扫描器为默认配置）。
func NewServer() *Server {
	return &Server{scanner: scanner.New(scanner.Config{}), done: make(chan struct{})}
}

// Done 返回退出信号通道（/api/exit 触发后关闭，供主流程等待退出）。
func (s *Server) Done() <-chan struct{} { return s.done }

// Shutdown 触发主流程退出（幂等，可安全多次调用）。
func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
	default:
		close(s.done)
	}
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
	mux.HandleFunc("/api/results/since", s.handleResultsSince)
	mux.HandleFunc("/api/stop", s.handleStop)
	mux.HandleFunc("/api/report", s.handleReport)
	mux.HandleFunc("/api/browse", s.handleBrowse)
	mux.HandleFunc("/api/exit", s.handleExit)
	mux.HandleFunc("/api/patterns", s.handlePatterns)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

type scanRequest struct {
	Path       string        `json:"path"`
	Recursive  bool          `json:"recursive"`
	MaxSize    int64         `json:"max_size"`
	Levels     []types.Level `json:"levels"`
	Workers    int           `json:"workers"`
	MaxResults int           `json:"max_results"`
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
	s.scanner = scanner.New(scanner.Config{
		MaxFileSize: req.MaxSize,
		ScanLevels:  req.Levels,
		Workers:     req.Workers,
		MaxResults:  req.MaxResults,
	})
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
	scanned, total, current := s.scanner.Progress()
	stats := s.scanner.Stats()
	pct := 0
	if total > 0 {
		pct = scanned * 100 / total
	}
	writeJSON(w, map[string]any{
		"progress":     pct,     // 兼容旧前端百分比
		"scanned":      scanned, // 已扫描文件数
		"total":        total,   // 已发现文件数（扫描中实时增长）
		"current_file": current,
		"scanning":     scanning,
		"stats":        stats, // 含 truncated_count / truncated_by_level
	})
}

func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	f := scanner.ResultFilter{
		Level:   types.Level(q.Get("level")),
		Type:    q.Get("type"),
		Keyword: q.Get("kw"),
	}
	items, total := s.scanner.ResultsPage(offset, limit, f)
	writeJSON(w, map[string]any{
		"results": items,
		"total":   total,
		"stats":   s.scanner.Stats(),
	})
}

// handleResultsSince 增量返回序号 seq 之后的新结果（扫描中轮询用，避免拉全量）。
func (s *Server) handleResultsSince(w http.ResponseWriter, r *http.Request) {
	seq, _ := strconv.Atoi(r.URL.Query().Get("seq"))
	items, nextSeq := s.scanner.ResultsSince(seq)
	writeJSON(w, map[string]any{
		"results":  items,
		"next_seq": nextSeq,
	})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.scanner.Stop()
	w.WriteHeader(http.StatusOK)
}

// handleExit 退出整个程序（解决 Windows GUI 关闭浏览器后进程残留、无法删除 exe 的问题）。
func (s *Server) handleExit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
	go func() {
		time.Sleep(150 * time.Millisecond) // 等响应写完再退出
		s.Shutdown()
	}()
}

// handlePatterns 返回当前内置的正则规则（供前端规则页展示，为后续增删改查铺路）。
func (s *Server) handlePatterns(w http.ResponseWriter, r *http.Request) {
	type patternInfo struct {
		Name        string      `json:"name"`
		Pattern     string      `json:"pattern"`
		Level       types.Level `json:"level"`
		Description string      `json:"description"`
		Examples    []string    `json:"examples"`
		CrossLine   bool        `json:"cross_line"`
	}
	all := patterns.All()
	out := make([]patternInfo, len(all))
	for i, p := range all {
		out[i] = patternInfo{
			Name:        p.Name,
			Pattern:     p.Pattern,
			Level:       p.Level,
			Description: p.Description,
			Examples:    p.Examples,
			CrossLine:   p.CrossLine,
		}
	}
	writeJSON(w, map[string]any{"patterns": out})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	format := report.Text
	if r.URL.Query().Get("format") == "json" {
		format = report.JSON
	}
	contentType := "text/plain; charset=utf-8"
	ext := "txt"
	if format == report.JSON {
		contentType = "application/json; charset=utf-8"
		ext = "json"
	}
	// Header 必须在写 body 前设好（流式输出后不能再改 Header）
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=scan-report-%s.%s", time.Now().Format("20060102"), ext))
	if format == report.Text {
		_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM（text 格式，避免中文乱码）
	}
	_ = report.GenerateTo(s.scanner, format, w) // 流式写 HTTP 响应，不累积全量字符串
}

// handleBrowse 目录浏览：供前端目录选择器懒加载子目录。
//
//	path 为空：Windows 返回盘符列表；macOS/Linux 返回 "/" 与主目录
//	path 为目录：返回其下的子目录（file=1 时同时返回文件，供后续选可执行文件）
//	无权限 / 不存在：HTTP 仍 200，在 error 字段返回友好提示
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := q.Get("path")
	wantFile := q.Get("file") == "1"

	type browseEntry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}
	resp := struct {
		Path    string        `json:"path"`
		Parent  string        `json:"parent"`
		Entries []browseEntry `json:"entries"`
		Error   string        `json:"error,omitempty"`
	}{Path: p}

	// 根入口（~ 展开为主目录）
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			p = home
		}
	}
	if p == "" {
		if runtime.GOOS == "windows" {
			for c := 'C'; c <= 'Z'; c++ {
				drive := string(c) + `:\`
				if fi, err := os.Stat(drive); err == nil && fi.IsDir() {
					resp.Entries = append(resp.Entries, browseEntry{Name: drive, Path: drive, IsDir: true})
				}
			}
		} else {
			resp.Entries = []browseEntry{{Name: "/", Path: "/", IsDir: true}}
			if home, err := os.UserHomeDir(); err == nil {
				resp.Entries = append(resp.Entries, browseEntry{Name: "~（主目录）", Path: home, IsDir: true})
			}
		}
		writeJSON(w, resp)
		return
	}

	resp.Path = p
	resp.Parent = parentDir(p)

	fi, err := os.Stat(p)
	if err != nil {
		resp.Error = "无法访问: " + err.Error()
		writeJSON(w, resp)
		return
	}
	if !fi.IsDir() {
		resp.Error = "不是目录: " + p
		writeJSON(w, resp)
		return
	}
	dirEntries, err := os.ReadDir(p)
	if err != nil {
		resp.Error = "读取目录失败: " + err.Error()
		writeJSON(w, resp)
		return
	}
	for _, e := range dirEntries {
		isDir := e.IsDir()
		if !wantFile && !isDir {
			continue // 默认只列子目录，避免文件过多撑爆列表
		}
		resp.Entries = append(resp.Entries, browseEntry{
			Name:  e.Name(),
			Path:  filepath.Join(p, e.Name()),
			IsDir: isDir,
		})
	}
	writeJSON(w, resp)
}

// parentDir 返回上级目录路径；已是根则返回空（前端据此隐藏“返回上级”）。
func parentDir(p string) string {
	sep := string(filepath.Separator)
	// Windows 盘符根（C:\）回到盘符列表
	if runtime.GOOS == "windows" && len(p) == 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		return ""
	}
	if p == "/" {
		return ""
	}
	d := filepath.Dir(p)
	if d == "." || d == p {
		return ""
	}
	// Windows: filepath.Dir("C:\Users") == "C:"，补回分隔符
	if runtime.GOOS == "windows" && len(d) == 2 && d[1] == ':' {
		return d + sep
	}
	return d
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
