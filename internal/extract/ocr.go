package extract

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrNoTesseract 表示未找到 tesseract 引擎，上层应静默跳过图片。
var ErrNoTesseract = errors.New("未找到 tesseract OCR 引擎，已跳过图片扫描")

// findTesseract 按优先级查找 tesseract 可执行文件：
//  1. exe 同目录的 tesseract/tesseract(.exe)（便携版）
//  2. Windows 常见安装位置（含原工具硬编码的 E:\OCR）
//  3. 系统 PATH
func findTesseract() (string, error) {
	name := tesseractName()

	// 1. 便携版：exe 同目录 tesseract/
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "tesseract", name)
		if fileExists(candidate) {
			return candidate, nil
		}
	}

	// 2. Windows 常见路径
	if runtime.GOOS == "windows" {
		for _, p := range []string{
			`E:\OCR\tesseract.exe`,
			`C:\Program Files\Tesseract-OCR\tesseract.exe`,
			`C:\Program Files (x86)\Tesseract-OCR\tesseract.exe`,
		} {
			if fileExists(p) {
				return p, nil
			}
		}
	}

	// 3. PATH
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", ErrNoTesseract
}

func tesseractName() string {
	if runtime.GOOS == "windows" {
		return "tesseract.exe"
	}
	return "tesseract"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Image 用 tesseract OCR 提取图片文字。优先 chi_sim+eng（简体+英文），失败回退 eng。
// 找不到引擎返回 ErrNoTesseract。
func Image(path string) (string, error) {
	bin, err := findTesseract()
	if err != nil {
		return "", err
	}
	if out, err := runTesseract(bin, path, "chi_sim+eng"); err == nil {
		return out, nil
	}
	// 回退纯英文
	return runTesseract(bin, path, "eng")
}

// runTesseract 调用 tesseract <image> stdout -l <lang>，返回识别文本。
func runTesseract(bin, image, lang string) (string, error) {
	cmd := exec.Command(bin, image, "stdout", "-l", lang)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
