//go:build gui

package web

import (
	"testing"
	"time"

	"sensitivescanner/internal/scanner"
	"sensitivescanner/internal/types"
)

func TestBroadRootCustomScanUsesFullDiskFastPolicy(t *testing.T) {
	req := scanRequest{
		Path:       `C:\`,
		Recursive:  false,
		Levels:     []types.Level{types.Critical, types.High, types.Medium, types.Low},
		WalkEngine: scanner.WalkEngineStd,
		Profile:    scanner.ProfileNormal,
		Workers:    16,
		MaxSize:    10 * 1024 * 1024,
	}
	cfg := scanConfig(req, []string{`C:\`})
	if cfg.ScanProfile != scanner.ProfileFullDiskFast {
		t.Fatalf("ScanProfile=%s want %s", cfg.ScanProfile, scanner.ProfileFullDiskFast)
	}
	if cfg.WalkEngine != scanner.WalkEngineFastwalk {
		t.Fatalf("WalkEngine=%s want fastwalk", cfg.WalkEngine)
	}
	if cfg.Workers != 4 {
		t.Fatalf("Workers=%d want 4", cfg.Workers)
	}
	if cfg.PerFileTimeout != 30*time.Second {
		t.Fatalf("PerFileTimeout=%s want 30s", cfg.PerFileTimeout)
	}
	if len(cfg.ScanLevels) != 2 || cfg.ScanLevels[0] != types.Critical || cfg.ScanLevels[1] != types.High {
		t.Fatalf("ScanLevels=%v want critical/high", cfg.ScanLevels)
	}
}
