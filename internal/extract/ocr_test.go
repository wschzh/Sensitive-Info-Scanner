package extract

import (
	"errors"
	"testing"
)

// TestImageNoTesseract 测试无 tesseract 环境下 Image 返回 ErrNoTesseract（静默跳过的依据）。
func TestImageNoTesseract(t *testing.T) {
	if _, err := findTesseract(); err == nil {
		t.Skip("环境已装 tesseract，跳过无引擎分支测试")
	}
	_, err := Image("any.png")
	if !errors.Is(err, ErrNoTesseract) {
		t.Errorf("无 tesseract 应返回 ErrNoTesseract，got %v", err)
	}
}
