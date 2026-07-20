LDFLAGS := -s -w
WIN_GUI_LDFLAGS := -s -w -H=windowsgui

.PHONY: mac-cli mac-gui win win-gui linux-x64 linux-x64-gui linux-arm64 linux-arm64-gui release test vet clean tidy help

# ==================== macOS ====================
mac-cli:
	go build -trimpath -ldflags="$(LDFLAGS)" -o dist/scanner-macos .
mac-gui:
	go build -trimpath -ldflags="$(LDFLAGS)" -tags gui -o dist/scanner-macos-gui ./cmd/gui

# ==================== Windows ====================
win:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/scanner-win.exe .
win-gui:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(WIN_GUI_LDFLAGS)" -tags gui -o dist/scanner-win-gui.exe ./cmd/gui

# ==================== Linux x86_64 ====================
linux-x64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/scanner-linux-x64 .
linux-x64-gui:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -tags gui -o dist/scanner-linux-x64-gui ./cmd/gui

# ==================== Linux ARM64（麒麟/统信 + 飞腾/鲲鹏 等国产系统）====================
linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/scanner-linux-arm64 .
linux-arm64-gui:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -tags gui -o dist/scanner-linux-arm64-gui ./cmd/gui

# ==================== 一次打包全平台图形界面版 ====================
release: win-gui mac-gui linux-x64-gui linux-arm64-gui
	@ls -lh dist/*

# ==================== 开发与维护 ====================
test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf dist/

tidy:
	go mod tidy

help:
	@echo "macOS:        make mac-cli | mac-gui"
	@echo "Windows:      make win | win-gui"
	@echo "Linux x64:    make linux-x64 | linux-x64-gui"
	@echo "国产 ARM64:    make linux-arm64 | linux-arm64-gui"
	@echo "全部 GUI:     make release"
	@echo "其他:         make test | vet | clean"
