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

## ✅ 2026-07-21 完成清单（第二批：1.3 / 2.7 / 2.8）

- **2.7** 大 xlsx OOM → `extract/xlsx.go` 改 `f.Rows` 流式迭代替代 `GetRows` 全量物化（避免 `[][]string` 双重驻留）；新增 `uncompressedSize` 用 `archive/zip` 累加解压后大小做二次保护（200MB 上限，绕过压缩前 `MaxFileSize` 对 zip 失效的问题）
- **2.8** Windows 路径误报 → 正则收紧为「至少两层目录」`[A-Za-z]:\\[^<>:"|?*\r\n \\]+\\[^<>:"|?*\r\n ]+`，过滤 `C:\temp` 这类单层裸串字面量；补 `TestWindowsPathRejectsSingleSegment` 反向用例
- **1.3** 结果删文件 → 前端每行加 checkbox + `data-path` + 表头全选 +「删除选中」按钮（原生 confirm 二次确认，删除成功本地即时移除行）；后端 `POST /api/delete`，安全约束：白名单只删当前结果集中路径 + 仅文件不目录（`os.Remove`），逐条返回结果

`go test ./...` 全绿，`go build -tags gui` + `go vet -tags gui ./internal/web/` 通过。浏览器实测通过：扫描 testdata 出 23 条（Windows 多层路径 `C:\Users\admin\app\config` / `D:\logs\app.log` 仍命中，2.8 无回归）；删文件端到端验证——白名单拒绝 `/etc/passwd` 等非结果集路径、真删临时文件后磁盘+结果行均消失、单选/全选按钮联动正常。

---

## 📋 下一阶段任务规划（2026-07-21 制定）

> C 盘全盘扫描卡顿专项优化计划见 `C_DRIVE_OPTIMIZATION_PLAN.md`。

剩余 6 项待办（1.3 / 1.4 / 1.5 / 2.7 / 2.8 / 2.10），方案已细化到「文件:行号」锚点。按价值/工作量/依赖分三梯队。

### 优先级矩阵

| 梯队 | 任务 | 价值 | 工作量 | 依赖 | 关键风险/决策 |
|---|---|---|---|---|---|
| 一 | **1.5** 规则 CRUD+持久化 | 高 | 大 | 无 | 包级 `all` 需加 RWMutex；配置位置【决策A】 |
| 一 | **1.4** OCR 外接配置化 | 高 | 中 | 无 | 测试图需 embed；保留 `Image` 旧签名兼容测试 |
| 二 | **2.10** 日志/误报白名单 | 高 | 中 | 可复用 1.5 持久化层 | `.log` 已排除，诉求需重定义【决策C】 |
| 二 | **1.3** 结果删文件 | 中 | 小 | 衔接 1.2 renderResults | 安全白名单【决策B】 |
| 三 | **2.8** Windows 路径误报 | 中 | 小 | 需先收误报样本 | 收紧正则可能漏报真阳性 |
| 三 | **2.7** 大 xlsx 流式 | 中 | 大 | 无 | matchContent 改造影响面大；MaxFileSize 已兜底，优先级低 |

### 待决策项（开工前拍板）

- **【决策A】规则/白名单配置文件位置**：① exe 同目录 `rules.json`（便携，与 `tesseract/` 约定一致）② `os.UserConfigDir()`（跨平台标准，macOS `.app` bundle 路径深）→ 建议 ①
- **【决策B】1.3 删文件安全边界**：仅允许删「当前结果集中出现的路径」+ 只删文件不删目录（`os.Remove` 非 `RemoveAll`）→ 建议默认开启白名单
- **【决策C】2.10 真正诉求**：`.log` 已在 `ExcludeExtensions`(patterns.go:227) 免扫；缺口是 ① 路径含 log 的文件（`logs/access.log`、`app.log.1`）② 其他误报大户（`.out/.err/.pid`）③ `IsExcludedDir` 未 ToLower，Windows `LOGS` 目录漏排 ④ 前端可配置 → 需确认范围

### 建议执行顺序

1. **1.5**（搭持久化层 + 前端可配置框架，2.10 复用）
2. **1.4**（独立，mac/Linux 刚需）
3. **2.10**（复用 1.5 配置机制）
4. **1.3**（衔接已有结果区）
5. **2.8 / 2.7**（收集样本后按需推进）

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

### 1.3 扫描结果支持选中后直接删除文件 ✅

- **现状**：`index.html:278-307` `renderResults` 拼 5 列 `<tr>`，`file_path` 仅落在「文件」列 `title` 属性（悬停可见），**未存 `data-*`、无 checkbox/选中机制**；后端无任何删文件代码（`os.Remove` 全项目零命中）。
- **方案**：
  - [ ] 前端：`renderResults` 每行加 `<td><input type="checkbox" class="row-sel" data-path="...">`（`file_path` 必须落 `data-path`）+ 表头全选 checkbox + 工具栏「删除选中(N)」按钮
  - [ ] 前端：复用 `.modal`（`index.html:126-138`）做确认弹层，列待删文件数 + 路径预览（原生 `confirm` 与 `exitApp` 风格不一致，统一到 modal）
  - [ ] 后端：`server.go` 新增 `POST /api/delete`，入参 `{paths: []string}`，照搬 `handleScan:95-141` 校验模板
  - [ ] 后端安全：① 仅删文件不删目录（`os.Stat` 校验 `!IsDir()` + `os.Remove` 非 `RemoveAll`）② **白名单**：只允许删出现在 `s.scanner.Results()` 中的路径，防前端伪造；逐条返回 `{path, ok, err}`
  - **涉及**：`internal/web/server.go`、`internal/web/index.html`

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
- **调查补充（2026-07-21）**：
  - ⚠️ `extract.Image` 不能改签名：`ocr_test.go:13` 直接调用，需新增 `ImageWith(path, OCRConfig)`，旧 `Image` 保留为薄封装
  - ⚠️ 测试图 `testdata/ocr.png`(29KB) 已存在但未 embed；`//go:embed` 无法嵌仓库根 testdata，需放 `internal/web/` 或 `internal/extract/` 下并 `//go:embed`
  - ✅ 后端 `/api/browse?file=1`(server.go:262) 已支持选文件，前端目录弹层(`index.html:309-369`)当前只渲染 `is_dir`，需加「文件模式」变体 `openBrowserFile(targetId)`
  - ✅ GUI 与 CLI 共享 `scanner.Config`，CLI flag 改动自动惠及 GUI；Web 表单需单独同步字段

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

### 2.7 [优化] 大 xlsx 内存占用 ✅
- **现状**：`extract/xlsx.go:13-35` 用 `excelize.OpenFile` + `f.GetRows(sheet)` **全量物化** `[][]string`，再 `strings.Builder` 拼大字符串（双峰值）；`MaxFileSize` 在 scanner.go:442 拦的是**压缩前**大小，对 zip(10-50x) 完全无效。
- **方案**：
  - [ ] `extract.XLSX` 改用 `f.Rows(sheet)` 流式迭代（excelize v2.11.0 原生支持）+ `SharedStringLoader` 惰性加载共享字符串
  - [ ] extract 层加解压后大小二次保护：`zip.OpenReader` 累加 `UncompressedSize64`，超阈值（如 200MB）跳过并记日志
  - [ ] 两选一：① extract 内部分批拼接仍返回整段字符串（简单，OOM 风险降但未除）② `matchContent` 改流式回调（彻底，但影响 xlsx/docx/pdf/文本全格式）→ 建议先①
  - [ ] 保持 `xlsx_test.go:12-42` 契约（返回值仍含手机号/身份证/邮箱）
  - **涉及**：`internal/extract/xlsx.go`、（方案②另涉）`internal/scanner/scanner.go`

### 2.8 [优化] Windows 路径可能误报 ✅

- **现状**：`patterns.go:125-131`「内部服务器路径」Low 级，正则 `[A-Za-z]:\\[^<>:"|?*\r\n ]+` 贪婪匹配，命中代码/日志字面量（如 `"C:\logs\app"`）；`matchContent`(scanner.go:477-499) **无任何上下文校验**（引号内/代码字面量）；`TestWindowsPathLazy`(patterns_test.go:53) 只测「能匹配」，缺反向「不应误报」用例；testdata 无误报样本。
- **方案**：
  - [ ] 先收集误报样本（真实扫描结果里挑 Windows 路径命中，归类：代码字面量 / 日志 / 配置）
  - [ ] 正则收紧：要求盘符路径后跟文件扩展名或 `/` `\`（如 `[A-Za-z]:\\[^...]+\.\w+`），或降低该规则级别
  - [ ] （可选）matchContent 加按 pattern name 分发的后处理钩子，引号内匹配降级/丢弃
  - [ ] 补反向单测；与原 Python 版行为对齐确认
  - **涉及**：`internal/patterns/patterns.go`、`internal/scanner/scanner.go`、`internal/patterns/patterns_test.go`

### 2.9 win下关闭web窗口后，进程未结束 ✅
- **方案**：Web 页面加「退出程序」按钮 → `POST /api/exit` → 主流程 `<-srv.Done()` 返回后退出，进程结束释放 exe（已实测优雅退出 exit 0）

### 2.10 [优化] 日志/误报文件白名单

- **现状（2026-07-21 复核修正）**：TODO 原文称「日志文件大量误扫」，但调查发现 `.log` **早已**在 `ExcludeExtensions`(patterns.go:227) 免扫、且 `.log` 不在 `FileCategories` 默认就不扫。真正缺口：① 路径含 log 关键字的文件（`logs/access-2024-01-01`、`app.log.1`、`*.log.*`）② 其他误报大户扩展名（`.out` `.err` `.pid` `.dat`）③ `IsExcludedDir`(patterns.go:224) 未 `ToLower`，Windows 下 `LOGS`/`Temp` 目录漏排 ④ 白名单硬编码，无运行时配置入口。
- **方案**：
  - [ ] `patterns`：扩展 `ExcludeExtensions`（`.out/.err/.pid/.dat` 等），`IsExcludedDir` 加 `strings.ToLower`
  - [ ] `scanner.shouldScan`：支持「路径分段含 log/out 等关键字」免扫（分段检查、大小写不敏感）
  - [ ] （依赖 1.5 持久化层）`scanRequest` + `Config` 加 `ExcludeGlobs []string` / `ExcludeKeywords []string`，前端扫描配置区加白名单输入框
  - **涉及**：`internal/patterns/patterns.go`、`internal/scanner/scanner.go`、（可选）`internal/web/server.go`、`internal/web/index.html`

### 2.11 [下版本] 旧版 Excel `.xls` 精准解析或明确跳过

- **现状**：`.xls` 在 `FileCategories["data"]` 可扫描列表里，但当前 `extract` 只实现了 `.xlsx`；`.xls` 会落到默认文本读取路径，把二进制内容当文本解码后跑正则，容易漏掉真实单元格内容，也可能产生乱码和误报。
- **决策项**：
  - [ ] 方案 A：接入支持 BIFF `.xls` 的解析库，提取单元格文本后进入统一规则匹配。
  - [ ] 方案 B：在全盘快速模式默认跳过 `.xls`，并在 Web/报告中提示“旧版 xls 暂不支持精准解析”；自定义扫描可保留开关。
- **建议**：优先方案 B 降噪保守处理；下个大版本再评估方案 A 的依赖体积、跨平台兼容和解析准确性。
- **涉及**：`internal/extract/`、`internal/scanner/scanner.go`、`internal/patterns/patterns.go`、`internal/web/index.html`

### 2.12 [下版本] 旧版 Word `.doc` 精准解析或策略化处理

- **现状**：`.docx` 已通过解压 `word/document.xml` 精准提取文本；旧版 `.doc` 仍在可扫描列表中，但没有专门解析器，会按普通文本读取二进制内容，结果不可靠。测试中 `.doc/.docx` 不是唯一卡顿原因，但 `.doc` 的支持语义需要明确。
- **决策项**：
  - [ ] 方案 A：接入旧版 Word `.doc` 解析能力，提取正文文本后进入统一规则匹配。
  - [ ] 方案 B：在全盘快速模式默认跳过 `.doc`，自定义扫描里提供“扫描旧版 Word（可能较慢/误报）”开关。
- **建议**：不要无提示地把 `.doc` 当普通文本扫；下版本至少应实现方案 B 的显式策略和提示。
- **涉及**：`internal/extract/`、`internal/scanner/scanner.go`、`internal/patterns/patterns.go`、`internal/web/index.html`
