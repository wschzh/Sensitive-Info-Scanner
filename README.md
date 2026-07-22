# 敏感信息扫描工具
**Sensitive Info Scanner**


扫描文件中的敏感信息（私钥、API 密钥、身份证、银行卡、手机号等），支持文本、Office 文档、PDF 和图片（OCR）。用 Go 编写，编译为单文件可执行程序，**一份代码可打包到 Windows、macOS、国产 ARM Linux（麒麟/统信）三平台**，目标机器无需安装任何运行时。

## 功能

- **13 类敏感信息识别**：私钥、API 密钥、数据库连接串、身份证、银行卡、密码、JWT、手机号、邮箱、IP、URL、Windows 路径等
- **4 级分类**：严重 / 高级 / 中级 / 低级
- **多格式**：文本与代码、xlsx、docx、pdf、图片（OCR）
- **多编码**：UTF-8 / GBK / GB18030 / Latin-1
- **两种界面**：命令行（含交互模式）+ Web 图形界面（浏览器内操作）
- **全文匹配**：能识别跨多行的私钥
- **全盘快速 / 自定义扫描**：全盘快速自动扫描可访问盘符，仅检查严重/高级问题；自定义扫描可选路径、遍历方式和扫描级别
- **富文档隔离解析**：PDF / DOCX / XLSX 通过一次性子进程完成提取和匹配，降低复杂文档导致主进程卡死或内存保护触发的风险
- **结果按文件聚合**：Web 端按文件展示扫描结果，单文件超过 10 条命中时显示 `10+`
- **扫描总结**：扫描结束后展示扫描数量、问题数量、级别/类型分布和问题集中文件
- **多格式报告**：支持 Text、JSON、Markdown、HTML、Excel(xlsx) 导出

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

Web 图形界面提供两种扫描方式：

- **全盘快速**：无需填写路径，自动扫描所有可访问盘符，默认只扫严重/高级问题。
- **自定义扫描**：选择文件或目录，可调整遍历方式、递归、最大文件大小和扫描级别。

扫描结果会实时刷新并按文件聚合展示。扫描结束后会显示汇总信息，支持导出 Text / Markdown / HTML / Excel / JSON 报告，也可以在确认后删除选中的问题文件。

### 命令行

```bash
scanner <路径>                                # 扫描，结果输出到控制台
scanner <路径> -o report.txt                  # 扫描并保存报告
scanner <路径> -f json -o r.json              # JSON 格式报告
scanner <路径> --level critical --level high  # 仅扫指定级别
scanner -i                                    # 交互模式
```

选项：`-recursive`（递归子目录）、`-o`（输出文件）、`-f text|json|md|html|xlsx`（格式）、`--max-size`（最大文件字节数）、`-level`（级别，可多选）。

## 诊断日志

稳定版本默认不生成 debug 日志，避免正常使用时在程序目录下留下 `scanner-debug.log`。

如需排查卡顿、超时或复杂富文档问题，可在启动前设置环境变量：

```bash
SCANNER_DEBUG=1
```

开启后日志会写到程序同目录的 `scanner-debug.log`。


## 敏感信息分级

| 级别 | 类型 |
|---|---|
| 严重 | 私钥、API 密钥、数据库连接串 |
| 高级 | 身份证、银行卡、密码、JWT |
| 中级 | 手机号、邮箱 |
| 低级 | 用户名、URL、Windows 路径、IP |

## 架构

本项目保持单文件部署：CLI 和 GUI 都编译为一个可执行文件，GUI 版本内嵌 Web 页面并监听本地 HTTP 服务。

核心流程：

1. Web/CLI 接收扫描请求，构造 `scanner.Config`。
2. 扫描器遍历文件系统，并按扩展名、目录、路径关键词、文件大小等规则预过滤。
3. 普通文本、代码、配置文件在主进程中解码并匹配规则。
4. PDF / DOCX / XLSX 富文档通过隐藏子进程 `--extract-worker` 完成提取和匹配，主进程只接收结构化命中结果。
5. Windows 下富文档 worker 会加入 Job Object，并设置内存限制、超时和输出上限，避免复杂文档拖垮主进程。
6. 扫描结果保存在内存中，Web 端按文件聚合分页展示；单文件超过 10 条命中时显示 `10+`。
7. 报告模块基于同一份结果生成 Text、JSON、Markdown、HTML 和 Excel。

debug 日志默认关闭；需要排障时通过 `SCANNER_DEBUG=1` 开启。

## 目录结构

```
├── main.go                  # 命令行入口
├── cmd/gui/main.go          # 图形界面入口（-tags gui 编译）
├── internal/
│   ├── patterns/            # 正则模式与文件分类
│   ├── scanner/             # 扫描核心与多编码解码
│   ├── extract/             # xlsx/docx/pdf/ocr 提取
│   ├── worker/              # 富文档隔离 worker
│   ├── workerproto/         # 主进程与 worker 的 JSON 协议
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
