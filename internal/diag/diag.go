// Package diag provides lightweight file diagnostics for GUI runs where stdout
// is not visible.
package diag

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	logPath string
)

// LogPath returns the diagnostic log path, creating the parent directory when
// possible. It falls back to the OS temp directory if the user cache directory
// is unavailable.
func LogPath() string {
	mu.Lock()
	defer mu.Unlock()
	if logPath != "" {
		return logPath
	}
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "SensitiveInfoScanner")
	_ = os.MkdirAll(dir, 0o755)
	logPath = filepath.Join(dir, "scanner-debug.log")
	return logPath
}

// Printf appends a single timestamped diagnostic line. Logging failures are
// intentionally ignored so diagnostics never affect scanning.
func Printf(format string, args ...any) {
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
