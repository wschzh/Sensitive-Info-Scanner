# 敏感信息扫描器架构复盘与后续方案

## 1. 当前目标

本项目的目标是提供一个极简、单文件部署的敏感信息扫描工具：

- 一个 Windows exe 即可运行。
- Web GUI 作为主要交互入口。
- 支持全盘快速扫描和自定义目录扫描。
- 支持文本、代码、配置、数据库、PDF、Word、Excel、图片等文件类型。
- 尽量降低误报、卡顿和崩溃风险。

当前开发重点已经从“能扫”进入到“全盘稳定扫、不卡死、可诊断、误报可控”的阶段。

## 2. 当前架构

### 2.1 入口层

项目主要入口：

- CLI：`main.go`
- GUI：`cmd/gui/main.go`

GUI 版本启动后会：

1. 启动本地 HTTP 服务。
2. 打开浏览器访问 Web 页面。
3. Web 页面通过 REST API 调用扫描、进度、结果分页、删除、导出等功能。

### 2.2 Web 层

核心文件：

- `internal/web/server.go`
- `internal/web/index.html`

当前 Web 层职责：

- 提供单页 Web GUI。
- 接收扫描请求。
- 根据扫描方式构造 scanner 配置。
- 提供进度接口 `/api/progress`。
- 提供分页结果接口 `/api/results`。
- 提供规则展示、报告导出、文件删除等接口。
- 扫描期间写 watchdog 日志。

当前前端扫描模式：

- 全盘快速：自动扫描所有可访问盘符，只扫严重/高级问题，使用 fastwalk。
- 自定义扫描：用户选择路径，可调整遍历方式、递归、最大文件、扫描级别。

### 2.3 扫描核心层

核心文件：

- `internal/scanner/scanner.go`

当前扫描核心流程：

1. `ScanPaths` 初始化扫描状态和 context。
2. producer 遍历文件系统，把可扫描文件送入 `fileCh`。
3. worker pool 从 `fileCh` 取文件。
4. 每个文件进入 `scanFileTimeout`。
5. `ScanFile` 调用 `readFile` 提取文本。
6. `matchContent` 对文本执行规则匹配。
7. aggregator 收集结果并更新统计。

当前已有保护：

- 文件大小上限 `MaxFileSize`。
- 提取后文本上限 `MaxTextSize`。
- 富文档原文件上限 `MaxRichFileSize`，默认跟随 `MaxFileSize`。
- 单文件超时 `PerFileTimeout`。
- worker 数上限。
- 富文档解析并发限制 `RichWorkers`。
- 超时积压限流 `timeout_backlog`。
- watchdog 内存保护。
- panic 捕获。
- 诊断日志。

### 2.4 规则层

核心文件：

- `internal/patterns/patterns.go`

当前规则特点：

- 按严重、高级、中级、低级分级。
- 支持 hints 预过滤。
- 支持数字候选过滤 `MinDigits`。
- 银行卡号已增加 Luhn 校验。
- 非银联裸号需要上下文，减少设备序列号等误报。

### 2.5 文本提取层

核心文件：

- `internal/extract/pdf.go`
- `internal/extract/docx.go`
- `internal/extract/xlsx.go`
- `internal/extract/ocr.go`

当前提取策略：

- `.docx`：解压并解析 `word/document.xml`。
- `.xlsx`：使用 excelize 流式读取 rows。
- `.pdf`：使用 `github.com/ledongthuc/pdf` 提取纯文本。
- 图片：依赖 OCR，当前全盘快速禁用图片扫描。
- `.doc` / `.xls`：目前没有精准解析能力，已记录为下版本问题。

## 3. 已完成的主要优化

### 3.1 全盘扫描模式收敛

已将 Web 端扫描模式收敛为：

- 全盘快速
- 自定义扫描

全盘快速默认：

- 扫所有可访问盘符。
- 只扫严重/高级。
- 使用 fastwalk。
- 禁用图片 OCR。
- 较短单文件超时。
- 较低 worker。
- 富文档解析并发为 1。

### 3.2 遍历优化

已引入 fastwalk，可在全盘快速模式下提升目录遍历速度。

同时保留标准遍历模式，因为 fastwalk 只解决“发现文件”的速度问题，不解决后续文件解析的风险。

### 3.3 结果与前端交互优化

已优化：

- 服务端分页。
- 扫描中结果保留。
- 后台刷新不覆盖手动筛选/翻页。
- 删除文件后同步移除后端结果。
- 进度接口返回活跃文件和最近事件。

### 3.4 误报优化

已做的代表性优化：

- Python 运行时目录噪音过滤。
- 日志、缓存、临时目录过滤。
- hints 预过滤。
- 数字候选过滤。
- 银行卡 Luhn 校验。
- 非银联裸号需要上下文。

### 3.5 诊断能力

已加入：

- `scanner-debug.log`。
- scan start / finish。
- walk start / finish。
- watchdog 心跳。
- active worker 文件。
- heap/sys/goroutine 指标。
- timeout / timeout_backlog / panic 事件。
- memory_guard_stop。

这让后续问题可以从“感觉卡死”变成“知道卡在哪里”。

## 4. 实测暴露的问题

### 4.1 全盘扫描仍会触发内存保护

多份日志显示，进程不是普通前端卡顿，而是扫描富文档时内存快速上涨。

典型日志特征：

- active worker 多数在 PDF/XLSX/DOCX。
- heap 从几十 MB 上升到数 GB。
- sys 内存更高。
- 最终触发 `memory_guard_stop`。

这说明当前瓶颈主要不在目录遍历，而在富文档解析。

### 4.2 fastwalk 不是根因

fastwalk 只影响“文件发现速度”。

它可能让文件更快进入扫描队列，从而更快暴露富文档解析压力，但它不是内存暴涨的直接原因。

切换到标准遍历可能缓解瞬时压力，但不能根治：

- PDF 解析仍可能 panic。
- XLSX 解析仍可能消耗大量内存。
- DOCX/PDF 提取后的文本仍可能很大。
- Go goroutine 超时后无法被强制杀掉。

### 4.3 Go goroutine 无法真正强杀

当前 `scanFileTimeout` 的超时机制只能让 worker 外层返回，但底层正在执行的 PDF/XLSX/DOCX 解析 goroutine 不能被强行终止。

结果是：

- 文件显示超时。
- worker 继续处理后续文件。
- 但后台解析 goroutine 可能仍在消耗 CPU/内存。
- 如果多个复杂富文档叠加，就会触发内存保护或进程卡死。

这是当前架构的核心限制。

### 4.4 富文档解析库风险较高

PDF 日志里已经出现 panic：

```text
panic=loading {5 0}: found {4 0}
github.com/ledongthuc/pdf.(*Reader).resolve
```

虽然当前已经 recover，不会直接让进程崩溃，但这说明 PDF 解析器对复杂/异常文件不够稳。

Excel 方面，虽然使用了流式 rows，但 excelize 打开复杂 xlsx 时仍可能有较大内存峰值。

### 4.5 内存保护解决的是“保命”，不是“扫完”

watchdog 内存保护可以避免 Windows 被拖死，但用户体验是扫描提前结束。

所以它只能作为最后防线，不能作为主要方案。

## 5. 当前瓶颈判断

按优先级排序，当前瓶颈是：

1. 富文档解析在主进程内执行，无法硬隔离。
2. goroutine 超时不可强杀。
3. PDF/XLSX/DOCX 解析器内存不可控。
4. 全盘场景富文档数量太多，复杂文件概率高。
5. Web/日志只是受害者，不是根因。
6. fastwalk 不是根因，只是更快触发压力。

## 6. 不建议的方案

### 6.1 仅切换标准遍历

不建议把“标准遍历”作为主要修复。

它可能降低文件进入速度，但无法解决解析器内存和 panic。

### 6.2 继续调低文件大小

调低最大文件可以降低风险，但会牺牲扫描覆盖面。

用户已经明确富文本仍然需要扫描，因此不能依赖简单跳过。

### 6.3 全盘快速跳过所有富文档

稳定性会提升，但功能不满足当前需求。

富文本仍然要扫，所以跳过只能作为可选策略，不能作为最终默认方案。

### 6.4 在 goroutine 内继续堆超时和 recover

已经证明不够。

recover 可以防 panic，timeout 可以让外层返回，但都不能释放正在执行的底层解析任务。

## 7. 修订后的下一阶段架构

### 7.1 总体修订

外部评审后的核心修订是：

> 富文档子进程不应只负责“提取文本”，而应在子进程内完成“提取 + 匹配”，只把少量结构化命中结果返回主进程。

原因是如果子进程把数 MB 纯文本交回主进程，仍会产生以下风险：

- 子进程持有一份文本。
- stdout 管道或 buffer 持有一份。
- 主进程读取后再持有一份。
- string/byte 转换可能继续复制。
- 主进程仍要承担大文本匹配内存峰值。
- stdout 消费不及时可能阻塞 worker。
- 敏感明文在 IPC 和日志中的暴露面扩大。

因此，修订后的可靠方案是：

```text
富文档子进程
  -> 分块/分页/逐行提取
  -> 在子进程内部立即执行规则匹配
  -> 丢弃已处理文本块
  -> 只返回 findings、统计和错误信息
```

主进程不再接收完整富文档文本。

### 7.2 推荐总架构

```text
Web GUI / REST API
        │
        ▼
Scan Manager
  ├── scan_id 状态机
  ├── 取消控制
  ├── 进度统计
  ├── 故障预算
  └── watchdog / pprof
        │
        ▼
Bounded Enumerator
  ├── fastwalk / 标准 walk
  ├── 有界文件队列
  ├── junction / reparse-point 控制
  └── 文件类型预检
        │
        ├──────────── 普通文本通道
        │                │
        │                ▼
        │         Streaming Matcher
        │                │
        │                ▼
        │
        └──────────── 富文档通道
                         │
                         ▼
                  Worker Supervisor
                         │
                         ▼
           Disposable Worker + Windows Job Object
             提取 -> 分块匹配 -> 返回 findings
                         │
                         ▼
                  Disk-backed Result Store
                         │
                         ▼
                 分页查询 / 导出 / 删除
```

### 7.3 主进程职责

主进程负责稳定性和调度，不直接承担高风险富文档解析：

- Web 服务。
- 扫描状态机。
- 文件遍历和有界队列。
- 普通文本/代码/配置文件的流式扫描。
- 富文档 worker 调度。
- worker 超时、kill、错误分类。
- 结果存储和分页查询。
- watchdog、pprof、日志。
- 删除文件审计和安全校验。

### 7.4 富文档 worker 职责

富文档 worker 是一次性子进程，负责单个高风险文件：

- 通过 stdin 接收请求。
- 读取文件并执行格式预检。
- 按页、行、sheet 或 XML token 流式提取。
- 在 worker 内部执行规则匹配。
- 只输出结构化 findings 和统计。
- 遇到 panic、超时、内存限制、解析错误时退出。

建议一文件一进程：

```text
一个 PDF/DOCX/XLSX
  -> 启动一个 worker
  -> 处理完成或失败
  -> worker 退出
```

优点：

- 解析后的 heap、缓存、库全局状态随进程释放。
- 第三方库泄漏不会传递到下一个文件。
- panic、死锁、OOM 只影响一个文件。
- 不需要证明解析库是否能安全复用。

后续如进程启动成本成为主要瓶颈，再考虑“每个 worker 处理 3-5 个文件后强制回收”，不建议直接做长期 worker pool。

### 7.5 单 EXE worker 形态

仍保持单文件部署。

同一个 exe 支持隐藏子命令：

```text
scanner-win-gui.exe --extract-worker
```

主进程通过 `os/exec` 启动自身作为 worker。

worker 启动后先阻塞等待 stdin 请求，不立即开始解析。

推荐启动顺序：

```text
主进程启动 --extract-worker
  -> worker 阻塞等待 stdin
  -> 主进程创建 Windows Job Object
  -> 配置 Job Object 内存和进程树限制
  -> 将 worker 加入 Job Object
  -> 主进程通过 stdin 发送文件路径和限制
  -> worker 开始解析和匹配
```

这样可以避免“worker 已经开始解析，但尚未加入 Job Object”的竞态。

### 7.6 Worker IPC 协议

主进程向 worker stdin 发送 JSON 请求：

```json
{
  "scan_id": "20260721-001",
  "file_id": 123,
  "path": "C:\\example\\a.pdf",
  "format": "pdf",
  "levels": ["critical", "high"],
  "timeout_ms": 30000,
  "max_findings": 1000,
  "max_output_bytes": 1048576,
  "mode": "full_disk_fast"
}
```

worker stdout 返回 JSON 响应：

```json
{
  "status": "completed",
  "file_id": 123,
  "findings": [
    {
      "rule_id": "bank_card",
      "level": "high",
      "location": "sheet1!B42",
      "masked_value": "6222****1234",
      "snippet": "银行卡：6222****1234"
    }
  ],
  "matched_count": 1,
  "truncated": false,
  "duration_ms": 1320
}
```

输出限制建议：

- stdout 最大 1MB 左右。
- 每个文件最多返回 1000 条 findings。
- 超出部分只记录计数和 `truncated=true`。
- stderr 单独限流，避免错误日志撑爆管道。

### 7.7 Windows Job Object 和内存限制

Windows Job Object 不应放到后续再考虑，应从 PDF 隔离第一版加入。

原因：

- `Process.Kill` 主要针对当前进程。
- 未来 OCR、外部 PDF 工具或 Office 解析器可能启动子进程。
- Job Object 可以管理整个进程树。
- Job Object 可设置进程或 Job 的提交内存上限。
- Job Object 关闭时可以清理关联进程。

建议初始组合：

```text
Job Object 硬限制：按机器和模式配置，例如 512MB 起步
GOMEMLIMIT 软限制：例如 384MB 起步
主进程超时：30 秒起步
stdout 上限：1MB 起步
stderr 上限：较小固定值
```

`GOMEMLIMIT` 只能作为 Go runtime 软内存限制，不能替代 Job Object 硬限制。

### 7.8 Worker 内部流式策略

不同格式分别处理。

DOCX：

- 先检查 ZIP entry 数、总解压大小、单 entry 大小、压缩比。
- 只解析必要 XML。
- 使用 XML token 流，不构建完整 document XML 字符串。
- 按段落或文本 run 匹配。

XLSX：

- 先检查 ZIP entry、`sharedStrings.xml` 大小、sheet 数。
- 按 sheet/row/cell 处理。
- 匹配后立即丢弃行数据。
- 限制最大 sheet 数、最大行列范围。

PDF：

- 尽可能按页提取。
- 按页匹配，处理完即丢弃页文本。
- 全盘快速模式可限制页数或文本量。
- 对已知 panic 文件类型和异常对象明确记录 skip reason。

图片/OCR：

- 视为更高风险队列。
- 默认禁用或极低并发。
- 后续单独设计。

数据库文件：

- 也应视为结构化高风险文件。
- 放入 worker 中只读打开。
- 避免锁定、修改或长时间占用用户数据库。

### 7.9 有界遍历和真实背压

fastwalk 仍然可用，但必须有真实背压。

建议：

```go
fileCh := make(chan FileTask, workerCount*2)
```

要求：

- 不把全部路径收集进 slice。
- 不使用超大 channel。
- 队列满时遍历器阻塞。
- 取消扫描后立即停止遍历。
- 默认不跟随 junction、symlink、reparse point。
- 检测目录循环。
- 网络盘、可移动盘、本地盘分别统计。

这样 fastwalk 只能按扫描器真实处理能力推进。

### 7.10 普通文本也逐步流式化

普通文本、代码、配置、日志不需要整文件读入内存。

建议后续改为 chunk 扫描：

```text
chunk 1: [0, 1MB]
chunk 2: [1MB - overlap, 2MB - overlap]
```

保留少量 overlap 处理跨块命中。

匹配结果只保留：

- rule ID。
- 文件偏移。
- 行号或近似位置。
- 脱敏片段。
- 去重键。

不要保留完整文本。

### 7.11 结果存储磁盘化

服务端分页只有在后端结果也磁盘化时，才能真正降低内存。

建议每次扫描创建临时 SQLite 数据库：

```text
%LOCALAPPDATA%\SensitiveScanner\scans\<scan-id>.db
```

建议表：

```text
scans
files
findings
events
worker_failures
```

收益：

- 分页查询不依赖内存完整结果。
- 大量结果不占主进程 heap。
- 扫描异常后仍可查看部分结果。
- 导出可直接从数据库流式生成。
- 筛选、去重、统计可以建索引。

单 EXE 部署不等于运行期间不能创建数据文件。若完全禁止落盘，就必须限制最大结果数，否则无法同时保证全盘扫描、完整结果和低内存。

### 7.12 扫描状态机

不要只有“扫描中/结束”。

建议状态：

```text
created
walking
scanning
cancelling
completed
completed_with_errors
degraded
stopped_by_memory_guard
failed
```

每次扫描使用唯一 `scan_id`。

所有进度、结果、取消、删除操作都绑定 `scan_id`，避免上一轮扫描 goroutine 更新下一轮状态。

### 7.13 故障预算和格式熔断

当某类格式连续失败时，不应停止整个扫描。

示例：

```text
PDF 连续 5 次崩溃
或最近 20 个 PDF 中超过 50% 超时
```

处理：

- 暂停 PDF 深度解析。
- 继续扫描普通文本和其他格式。
- 记录 `pdf_parser_degraded`。
- 前端提示“PDF 解析已降级”。

这比直接 `memory_guard_stop` 更友好。

### 7.14 删除接口安全补强

当前 Web 有删除文件能力，后续结果磁盘化后应强化：

- 前端只提交 finding ID 或 file ID。
- 后端从数据库解析真实路径。
- 确认路径属于本次扫描根目录。
- 删除前重新 `Lstat`。
- 不跟随 symlink/reparse point。
- 检查文件是否在扫描后被替换。
- 所有删除操作写审计日志。
- 本地 HTTP 服务绑定 `127.0.0.1`。
- 每次启动生成 API token，并校验 Host 和 Origin。

## 8. 需要补充的证据

当前日志能证明富文档和内存暴涨高度相关，但还不能严格证明所有内存都来自解析器。

重构前建议加入 pprof 和阶段指标。

### 8.1 自动保存 profile 的时机

建议自动保存：

1. 扫描开始。
2. 主进程内存达到警戒值。
3. `memory_guard_stop` 前。
4. 连续处理若干大型 XLSX/PDF 后。

profile 类型：

- heap。
- allocs。
- goroutine。
- CPU，必要时开启。

### 8.2 阶段耗时和内存指标

每个文件记录：

- open。
- precheck。
- extract。
- match。
- aggregate。
- worker wait。
- queue wait。

全局记录：

- 文件队列长度。
- 结果数。
- snippet 总字节数。
- active worker。
- 主进程 Go heap。
- 主进程 RSS / Private Bytes，Windows 下需单独采集。

这样才能验证子进程化以后，主进程是否仍有独立内存问题。

## 9. 修订后的实施顺序

### P0：补证据，不立即大改

- 增加 heap/allocs/goroutine profile。
- 增加阶段耗时和内存指标。
- 记录队列长度、结果数、snippet 字节数。
- watchdog 触发前自动保存 profile。
- 区分 Go heap、RSS、Private Bytes。

### P1：PDF 一文件一子进程

第一阶段先处理 PDF，因为 PDF 已经明确出现 panic。

必须同时包含：

- worker stdin 请求协议。
- worker 内匹配。
- Job Object。
- 硬超时。
- GOMEMLIMIT。
- stdout/stderr 上限。
- 只回传 findings。
- 错误类型和 skip reason。

### P2：迁移 DOCX/XLSX

- 加入 ZIP 预检。
- worker 内流式匹配。
- 不回传完整文本。
- 明确加密、损坏、不支持格式的 skip reason。

### P3：磁盘结果存储和扫描状态机

- SQLite 结果库。
- `scan_id`。
- 完整状态机。
- 前端展示 completed/degraded/stopped/failed。

### P4：异常文件语料库

建立测试语料：

- 损坏 PDF。
- 循环引用或异常对象 PDF。
- 数万页 PDF。
- 超大 `sharedStrings.xml`。
- ZIP 高压缩比文件。
- 数万个 ZIP entries。
- 加密 Office 文件。
- 超长路径。
- 无权限文件。
- junction 循环。
- 正在被其他程序修改的文件。
- 扫描过程中被删除或替换的文件。

## 10. 当前结论

原方案中“富文档解析必须进程隔离”的判断是正确的，但需要进一步修订：

- 不能让 worker 把完整提取文本交回主进程。
- worker 应在子进程内完成提取和匹配。
- 主进程只接收少量结构化 findings。
- Windows Job Object 应从第一版 PDF 隔离开始加入。
- `GOMEMLIMIT` 只是软限制，不能替代 Job Object。
- 文件队列、结果存储、普通文本扫描也需要继续收敛内存。
- 扫描状态需要从“扫描中/结束”升级为完整状态机。

最终推荐路线：

1. 主进程负责调度、普通文本扫描、状态和结果管理。
2. 每个富文档使用一次性子进程。
3. 子进程受 Job Object、超时、输出上限保护。
4. 子进程内部完成提取和匹配。
5. 普通文本和富文档都逐步流式处理。
6. 文件队列有界，形成真实背压。
7. 结果使用 SQLite 等磁盘存储。
8. 通过 pprof 验证主进程是否仍存在独立内存问题。
9. 单个解析器连续失败时降级该格式，而不是停止整个扫描。
10. 前端明确区分完成、部分完成、降级和资源保护停止。

这套方案比“子进程提取文本后交回主进程”更稳，也更符合全盘扫描、富文档扫描和单 EXE 部署三者同时成立的目标。
