# 敏感信息扫描工具
**Sensitive Info Scanner**

**作者**：cz

扫描文件中的敏感信息（私钥、API 密钥、身份证、银行卡、手机号等），支持文本、Office 文档、PDF 和图片（OCR）。用 Go 编写，编译为单文件可执行程序，**一份代码可打包到 Windows、macOS、国产 ARM Linux（麒麟/统信）三平台**，目标机器无需安装任何运行时。

## 功能

- **13 类敏感信息识别**：私钥、API 密钥、数据库连接串、身份证、银行卡、密码、JWT、手机号、邮箱、IP、URL、Windows 路径等
- **4 级分类**：严重 / 高级 / 中级 / 低级
- **多格式**：文本与代码、xlsx、docx、pdf、图片（OCR）
- **多编码**：UTF-8 / GBK / GB18030 / Latin-1
- **两种界面**：命令行（含交互模式）+ Web 图形界面（浏览器内操作）
- **全文匹配**：能识别跨多行的私钥

## 打包

环境要求：Go 1.25+

打包命令：

```bash
make win-gui          # Windows 图形界面   → dist/scanner-win-gui.exe
make win              # Windows 命令行      → dist/scanner-win.exe
make mac-gui          # macOS 图形界面      → dist/scanner-macos-gui
make linux-arm64-gui  # 国产 ARM64 图形界面 → dist/scanner-linux-arm64-gui
make linux-x64-gui    # Linux x86_64 图形界面
```

一次打包全平台图形界面版：

```bash
make release
```

产物均在 `dist/`，拷到对应系统直接运行。

等价的手动命令（以 Windows GUI 为例，其他平台只换 `GOOS/GOARCH`）：

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w -H=windowsgui" -tags gui \
  -o dist/scanner-win-gui.exe ./cmd/gui
```

| 目标系统 | GOOS | GOARCH |
|---|---|---|
| Windows | windows | amd64 |
| macOS (Apple Silicon) | darwin | arm64 |
| 国产 ARM64（麒麟/统信 + 飞腾/鲲鹏） | linux | arm64 |
| Linux x86_64 | linux | amd64 |

## 使用

### 图形界面（推荐）

- **Windows**：双击 `scanner-win-gui.exe` → 自动打开浏览器
- **macOS / Linux**：先 `chmod +x scanner-*-gui`，再运行 → 自动打开浏览器

在网页里填写扫描路径、勾选级别、点「开始扫描」，结果实时刷新，可一键导出 Text/JSON 报告。

### 命令行

```bash
scanner <路径>                                # 扫描，结果输出到控制台
scanner <路径> -o report.txt                  # 扫描并保存报告
scanner <路径> -f json -o r.json              # JSON 格式报告
scanner <路径> --level critical --level high  # 仅扫指定级别
scanner -i                                    # 交互模式
```

选项：`-recursive`（递归子目录）、`-o`（输出文件）、`-f text|json`（格式）、`--max-size`（最大文件字节数）、`-level`（级别，可多选）。

## OCR（图片文字识别）

图片扫描依赖 Tesseract OCR：

1. 安装 Tesseract（Windows：<https://github.com/UB-Mannheim/tesseract/wiki>）
2. 安装时勾选简体中文 `chi_sim` 语言包
3. 将 `tesseract` 加入 PATH，或放到程序同目录的 `tesseract/` 子目录

未安装时自动跳过图片，不影响其他格式扫描。

## 敏感信息分级

| 级别 | 类型 |
|---|---|
| 严重 | 私钥、API 密钥、数据库连接串 |
| 高级 | 身份证、银行卡、密码、JWT |
| 中级 | 手机号、邮箱、IP |
| 低级 | 用户名、URL、Windows 路径 |

## 目录结构

```
├── main.go                  # 命令行入口
├── cmd/gui/main.go          # 图形界面入口（-tags gui 编译）
├── internal/
│   ├── patterns/            # 正则模式与文件分类
│   ├── scanner/             # 扫描核心与多编码解码
│   ├── extract/             # xlsx/docx/pdf/ocr 提取
│   ├── report/              # 报告生成
│   ├── cli/                 # 交互模式
│   ├── types/               # 共享数据结构
│   └── web/                 # Web 图形界面（HTTP + 内嵌前端）
├── Makefile
└── go.mod
```

## 开发

```bash
make mac-cli     # 本地编译运行
make test        # 单元测试
make vet         # 静态检查
```

新增敏感信息模式只需编辑 `internal/patterns/patterns.go` 一处。
