// Package diag provides lightweight file diagnostics for GUI runs where stdout
// is not visible.
package diag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	logPath string
)

// Enabled reports whether file diagnostics are enabled. Stable GUI builds keep
// this off by default to avoid creating scanner-debug.log during normal use.
func Enabled() bool {
	return os.Getenv("SCANNER_DEBUG") == "1" || strings.EqualFold(os.Getenv("SCANNER_DEBUG"), "true")
}

// LogPath returns the diagnostic log path next to the executable so Windows GUI
// users can find it without digging through AppData. It falls back to the
// working directory and then the OS temp directory if needed.
func LogPath() string {
	mu.Lock()
	defer mu.Unlock()
	if logPath != "" {
		return logPath
	}
	base := ""
	if exe, err := os.Executable(); err == nil && exe != "" {
		base = filepath.Dir(exe)
	}
	if base == "" {
		base, _ = os.Getwd()
	}
	if base == "" {
		base = os.TempDir()
	}
	logPath = filepath.Join(base, "scanner-debug.log")
	return logPath
}

// Printf appends a single timestamped diagnostic line. Logging failures are
// intentionally ignored so diagnostics never affect scanning.
func Printf(format string, args ...any) {
	if !Enabled() {
		return
	}
	line := fmt.Sprintf(format, args...)
	path := LogPath()
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), line)
}
