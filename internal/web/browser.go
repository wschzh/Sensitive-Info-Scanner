//go:build gui

package web

import (
	"os/exec"
	"runtime"
)

// OpenBrowser 用系统默认浏览器打开 url。
// Windows 用 rundll32（"url.dll,FileProtocolHandler" 是单个逗号分隔参数）；
// macOS 用 open；Linux 用 xdg-open。
func OpenBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	return exec.Command(cmd, args...).Start()
}
