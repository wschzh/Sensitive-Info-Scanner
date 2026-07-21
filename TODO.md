# TODO — 敏感信息扫描工具（Go 版）

**作者**：cz　**创建日期**：2026-07-20　**最近更新**：2026-07-21

图例：`- [ ]` 待办　`- [x]` 已完成　`~` 进行中

---

## ✅ 2026-07-21 完成清单（方案A：Bug 清扫 + 结果体验）

- **2.1** Web 导出报告中文乱码 → 抽出 `report.WithBOM()`，`handleReport` 在 text 格式补 BOM
- **2.6** 导出文件名随机 → 加 `Content-Disposition: scan-report-YYYYMMDD.{txt,json}`
- **2.3** UTF-8 BOM 文件首行偏移 → `decodeText` 先 `TrimPrefix` BOM，补 2 条单测
- **2.2** OCR 子进程无超时 → `runTesseract` 用 `context.WithTimeout(60s)` + `CommandContext`
- **1.2 / 2.4 / 2.5** 结果区重构 → 全局 `allResults` + `renderResults()`（级别/类型/关键字筛选 + 每页 200 分页 + DocumentFragment）+ `poll()` 扫描中每 ~1s 增量拉 `/api/results`
- **1.1** 目录选择 → 后端 `GET /api/browse`（盘符 / 根 / 主目录 + 子目录懒加载 + parent + 友好错误）+ 前端目录选择弹层（懒加载 / 返回上级 / 回填 / 空目录提示）

浏览器实测全部通过：扫描 testdata 出 23 条、级别筛选(4)、搜索(root→1)、分页(250→2 页 200+50)、目录导航 / 回填 / 端到端扫描。`go test ./...` 全绿，`go build -tags gui` 通过。

---

## 一、Web UI 功能增强

### 1.1 目录选择（替代手输路径）✅
- **现状**：`internal/web/index.html:36` 是 `<input type="text" id="path">`，需用户手敲绝对路径，易错、体验差。
- **约束**：浏览器安全策略下 `<input type="file" webkitdirectory>` 与 File System Access API（`showDirectoryPicker`）**都拿不到磁盘绝对路径**，前端单方面无法解决，必须走后端。
- **方案**：后端新增目录浏览 API，前端做轻量目录选择器。
  - [ ] 后端：`server.go` 新增 `GET /api/browse?path=`
    - 入参为空：Windows 返回盘符列表（`C:\` `D:\` …）；macOS/Linux 返回 `/`、`~`（用户主目录）
    - 入参为目录：返回该目录下的**子目录**（仅目录，不返回文件，避免列表爆炸），供逐级展开
    - 无权限 / 路径不存在时返回友好错误 JSON
  - [ ] 后端：新增 `GET /api/browse?path=&file=1` 支持选文件（1.3 OCR 路径复用）
  - [ ] 前端：路径框旁加「浏览…」按钮 → 弹层显示目录树（懒加载子目录），点「选择此目录」回填绝对路径到 `#path`
  - [ ] 保留手输框作为回退（粘贴路径仍可用）
  - **涉及**：`internal/web/server.go`、`internal/web/index.html`

### 1.2 结果按级别筛选（扫描后实时过滤，不重扫）✅
- **现状**：顶部 `.lvl` 复选框是**扫描前**过滤（传入 `scanRequest.levels`）；扫描完成后 `loadResults()` 全量渲染表格，无法在不重扫的前提下切换查看级别。
- **方案**：纯前端过滤，无需后端改动。
  - [ ] `loadResults()` 把后端返回的 `r.results` 存到全局 `allResults`，不再直接渲染
  - [ ] 新增 `renderResults()`：按当前选中的级别 + 类型 + 搜索关键字过滤 `allResults` 后渲染
  - [ ] 结果区上方加一组级别 toggle（含「全部」）+ 类型下拉 + 搜索框，`change` 时调 `renderResults()`
  - [ ] 显示「当前 N / 共 M 条」计数
  - **涉及**：`internal/web/index.html`

### 1.3 扫描结果支持选中后直接删除文件

### 1.4 外接 OCR 支持（指定引擎路径 / 语言 / 开关）

- **现状**：`internal/extract/ocr.go` 的 `findTesseract()` 无参、路径硬编码（exe 同目录 `tesseract/` → `E:\OCR` → `Program Files\Tesseract-OCR` → `(x86)` → PATH）；`Image(path)` 无配置入口；`runTesseract` 写死 `chi_sim+eng`→`eng`。用户无法指定自定义路径或语言，`E:\OCR` 这种写死盘符在 mac/Linux 无意义。
- **方案**：把 OCR 配置做成可注入。
  - [ ] `scanner.Config` 新增字段：`TesseractPath string`（空则走原 findTesseract 链）、`OCRLang string`（默认 `chi_sim+eng`）、`EnableOCR *bool`（nil = 默认开，区分「未设置」与「显式关闭」）
  - [ ] `readFile` 把 OCR 配置透传给 `extract.Image`
  - [ ] `extract.Image` 改签名接收配置，或新增 `extract.ImageWith(path, cfg)`，旧 `Image` 保留为薄封装兼容
  - [ ] `findTesseract` 增加「用户指定路径优先，命中即用，否则回落原链」
  - [ ] CLI：`main.go` 加 `-tesseract <path>`、`-ocr-lang <lang>`、`-no-ocr` flag，透传到 Config
  - [ ] Web：扫描配置区加「OCR 设置」折叠面板
    - tesseract 路径输入框 +「浏览…」（复用 1.1 的 `/api/browse?file=1`）+「测试」按钮
    - 语言下拉：简中+英文 / 纯简中 / 纯英文 / 自定义
    - 「启用 OCR」复选框
  - [ ] 后端新增 `POST /api/ocr/test`：用配置的引擎跑一张内置极简测试图，返回识别文本或错误，让用户验证配置是否生效
  - [ ] `scanRequest` 增加 `tesseract_path` / `ocr_lang` / `enable_ocr` 字段，`handleScan` 透传到 `scanner.Config`
  - **涉及**：`internal/scanner/scanner.go`、`internal/extract/ocr.go`、`main.go`、`internal/web/server.go`、`internal/web/index.html`

### 1.5 可直接在web页面查看检索的正则规则，并支持自定义新增、删除、修改等功能
- **进度**：✅ 规则查看页已完成（「规则」tab + `/api/patterns`，按级别分组展示 13 条规则的名称/级别/正则/描述/示例/跨行标记）
- **待办**：增删改查（需为 patterns 引入持久化层：JSON 配置文件加载 + 热更新，目前规则为编译期硬编码）

---

## 二、Bug 与优化

### 2.1 [Bug] Web 导出报告中文乱码（缺 BOM）✅
- **现状**：`handleReport`（`server.go:137`）直接 `w.Write([]byte(content))`，未加 UTF-8 BOM；而写文件路径 `report.go` 有 `writeFileWithBOM`。Windows 记事本打开导出的 `.txt` 中文乱码。
- **方案**：`handleReport` 在 text 格式时先写 `0xEF 0xBB 0xBF`；把 BOM 逻辑抽成 `report.WithBOM(content) []byte` 供两处复用。
- **涉及**：`internal/web/server.go`、`internal/report/report.go`

### 2.2 [Bug] OCR 子进程无超时 ✅
- **现状**：`runTesseract` 用 `cmd.Output()` 无超时。损坏 / 超大图片可能让 tesseract hang 住，卡死整个扫描 goroutine。
- **方案**：`context.WithTimeout`（如 60s）包裹 `exec.CommandContext`，超时杀子进程并静默跳过该图。
- **涉及**：`internal/extract/ocr.go`

### 2.3 [Bug] UTF-8 BOM 文件首行偏移可能错位 ✅
- **现状**：`decodeText` 对 `utf8.Valid` 的原文直接当文本，未剥离 BOM（`EF BB BF`）。`computeLineStarts` 按字节算偏移，BOM 占首行前 3 字节，可能导致首行匹配内容 / 行号轻微错位。
- **方案**：`decodeText` 在 utf8 分支前 `bytes.TrimPrefix(raw, "\xEF\xBB\xBF")`，并补一条单测验证首行命中行号正确。
- **涉及**：`internal/scanner/encoding.go`、`internal/scanner/encoding_test.go`

### 2.4 [优化] 扫描中看不到增量结果 ✅
- **现状**：`poll()` 只更新进度条，扫描结束才调 `loadResults()`。大目录扫描时用户长时间盯着空表格。
- **方案**：`poll()` 每 N 次（如每 1s）也拉一次 `/api/results` 实时刷新结果表；`Results()` 已是并发安全拷贝可直接用。
- **涉及**：`internal/web/index.html`（`scanner.go` 已支持，无需改）

### 2.5 [优化] 大结果集渲染卡顿 ✅
- **现状**：`loadResults()` 用字符串拼接 + 一次性 `innerHTML`，几千条结果时浏览器卡顿。
- **方案**：分页（每页 200 条 + 翻页）或 `DocumentFragment` 批量插入；与 1.2 筛选合并实现。
- **涉及**：`internal/web/index.html`

### 2.6 [优化] 导出文件名随机 ✅
- **现状**：`handleReport` 未设 `Content-Disposition`，浏览器下载名随机（如 `download`）。
- **方案**：`w.Header().Set("Content-Disposition", "attachment; filename=scan-report-<日期>.<ext>")`。
- **涉及**：`internal/web/server.go`

### 2.7 [优化] 大 xlsx 内存占用
- **现状**：`extract.XLSX` 用 `excelize.GetRows` 全量读入。`readFile` 虽有 `MaxFileSize` 拦截，但压缩比高的 xlsx 解压后仍可能 OOM。
- **方案**：流式迭代（`excelize.Rows` 逐行读），或对解压后估算大小做二次保护。优先级低。
- **涉及**：`internal/extract/xlsx.go`

### 2.8 [优化] Windows 路径可能误报

- **现状**：模式 `[A-Za-z]:\\[^<>:"|?*\r\n ]+` 会命中代码 / 日志里的字符串字面量（如 `"C:\logs\app"`）。
- **方案**：评估是否要求路径后跟文件扩展名、或加常见日志 / 临时目录白名单；先收集误报样本再定。需与原 Python 版行为对齐确认。
- **涉及**：`internal/patterns/patterns.go`

### 2.9 win下关闭web窗口后，进程未结束 ✅
- **方案**：Web 页面加「退出程序」按钮 → `POST /api/exit` → 主流程 `<-srv.Done()` 返回后退出，进程结束释放 exe（已实测优雅退出 exit 0）

