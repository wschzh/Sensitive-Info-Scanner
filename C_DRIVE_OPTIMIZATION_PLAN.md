# C 盘全盘扫描卡顿优化计划

创建日期：2026-07-21  
分支：`codex/c-drive-scan-optimization`

## 目标

解决 Windows 下扫描整个 `C:\` 时出现的卡顿、进度反馈不明显、误报过多和资源占用偏高问题。

优化目标按优先级排序：

1. 减少无价值扫描面：默认跳过系统目录、缓存目录、日志目录和低价值扩展名。
2. 让全盘扫描更可控：提供“全盘快速”和“自定义扫描”两种入口，避免用户无意中启用 OCR、低级规则和图片扫描。
3. 提升遍历效率：评估并替换为 `fastwalk`，降低大目录遍历开销。
4. 降低正则成本：先做关键词预过滤，再运行正则匹配。
5. 改善 Web 体验：显示跳过统计、扫描速率和当前阶段，避免用户感觉程序卡死。

## 性能瓶颈判断

当前卡顿大概率不是单一正则慢，而是多因素叠加：

- `C:\` 下文件数量巨大，权限错误、系统目录和缓存目录很多。
- 低级规则（URL、IP、路径、用户名）在日志、缓存、代码产物里命中量很高。
- 图片 OCR 和 Office/PDF 提取属于慢路径，全盘扫描时容易拖住 worker。
- 当前遍历器是标准库 `filepath.WalkDir`，可用但不是为极大目录树优化。
- Web 端如果只看百分比，`total` 又在遍历中持续增长，会显得进度停滞。

## 阶段一：扫描面收缩（优先实现）

### 1.1 扩展默认排除目录

- [x] Windows 系统目录默认跳过：
  - `Windows`
  - `Program Files`
  - `Program Files (x86)`
  - `ProgramData`
  - `$Recycle.Bin`
  - `System Volume Information`
  - `Recovery`
  - `PerfLogs`
- [x] 用户目录下缓存默认跳过（通过 `cache/temp/tmp` 路径分段覆盖）：
  - `AppData\Local\Temp`
  - `AppData\Local\Microsoft\Windows\INetCache`
  - `AppData\Local\Google\Chrome\User Data\*\Cache`
  - `AppData\Local\Microsoft\Edge\User Data\*\Cache`
- [x] 开发/构建目录默认跳过：
  - `node_modules`
  - `.git`
  - `dist`
  - `build`
  - `target`
  - `.gradle`
  - `.m2`
  - `.nuget`

验收标准：

- `patterns.IsExcludedDir` 大小写不敏感。
- 新增单测覆盖 Windows 典型大小写变体。
- 扫描测试目录时，被排除目录内文件不计入 discovered。

### 1.2 支持路径分段关键词排除

- [x] 新增 `scanner.Config.ExcludePathKeywords []string`。
- [x] 默认关键词：`log`, `logs`, `cache`, `temp`, `tmp`。
- [x] 使用路径分段匹配，避免误伤普通文件名的一部分。
- [x] 大小写不敏感。

验收标准：

- `C:\Users\a\AppData\Local\Temp\x.txt` 被跳过。
- `C:\work\catalog\config.txt` 不因包含 `log` 字符串被误跳过。

### 1.3 扩展低价值扩展名排除

- [x] 默认跳过：
  - `.log`
  - `.log.1` / `.log.*`
  - `.tmp`
  - `.cache`
  - `.pid`
  - `.out`
  - `.err`
  - `.dat`
  - `.bin`
  - `.dll`
  - `.exe`
  - `.msi`
- [x] 对复合扩展名做特殊处理，如 `app.log.1`。

验收标准：

- `app.log.1`、`access.LOG.20260721` 默认跳过。
- `.bak` 备份文件可能包含历史配置和密钥，不默认跳过。
- 已支持的文档、代码、配置扩展名不回归。

## 阶段二：全盘扫描预设

### 2.1 新增扫描模式

- [x] Web 简化为两种扫描方式：
  - `全盘快速`：无需选择目录，自动扫描所有可访问盘符，固定快速遍历，只扫严重/高级。
  - `自定义扫描`：需要选择扫描路径，可调整扫描速度和扫描级别，默认严重/高级。
- [x] `scanner.Config.ScanProfile string` 保留：
  - `normal`：自定义扫描默认行为。
  - `full_disk_fast`：全盘快速模式。
- [x] CLI 保持兼容：`-profile normal|full_disk_fast|deep`。

模式建议：

| 模式 | 级别 | OCR | 图片 | 文档 | 低级规则 |
|---|---|---|---|---|---|
| 全盘快速 | 严重+高级 | 关 | 关 | 开 | 关 |
| 自定义扫描 | 用户选择，默认严重+高级 | 默认开 | 开 | 开 | 用户选择 |

验收标准：

- 选择 `全盘快速` 时，默认只扫严重/高级，且不要求扫描路径。
- Web 文案明确说明全盘快速会自动扫描所有可访问盘符。
- CLI 和 Web 使用同一套 profile 解析逻辑。

### 2.2 增加跳过统计

- [x] `ScanStatistics` 增加：
  - `SkippedFiles`
  - `SkippedDirs`
  - `SkippedByReason map[string]int`
- [x] `shouldScan` 返回跳过原因，而不是简单 bool。
- [x] Web 进度区显示跳过数量。

实现补充：JSON/Text 报告已输出跳过统计，后端统计会记录 `skipped_by_reason`。

跳过原因建议：

- `excluded_dir`
- `excluded_ext`
- `unsupported_ext`
- `too_large`
- `non_regular`
- `profile_disabled`

验收标准：

- Web 显示“已扫描/已发现/已跳过/当前文件”。
- JSON 报告包含跳过统计。

## 阶段三：遍历器优化

### 3.1 引入 fastwalk 实验开关

- [x] 评估库：`github.com/charlievieth/fastwalk` 或维护活跃的同类库。
- [x] 新增 `scanner.Config.WalkEngine string`：
  - `std`
  - `fastwalk`
- [x] 初期默认仍为 `std`；Web 显示为“标准/快速”，CLI 可开启 `fastwalk`。
- [ ] Windows 下重点测试权限错误、符号链接、junction、长路径。

验收标准：

- `go test ./...` 通过。
- 对测试目录树，`std` 和 `fastwalk` 发现的可扫描文件数量一致。
- 权限错误不终止扫描。

### 3.2 基准测试

- [x] 新增 `internal/scanner/walk_benchmark_test.go`。
- [x] 构造多层目录和大量小文件场景。
- [x] 对比：
  - 遍历耗时
  - discovered 数量
  - skipped 数量
  - 内存分配

验收标准：

- 有稳定 benchmark 命令：

```bash
go test -bench=Walk -benchmem ./internal/scanner
```

- README 或 TODO 中记录一组本机对比结果。

本机 macOS 初测：

```text
BenchmarkWalkEngineStd-8         5062593 ns/op  1202275 B/op  11836 allocs/op
BenchmarkWalkEngineFastwalk-8    4966164 ns/op  1142300 B/op  11368 allocs/op
```

结论：macOS 小样本仅小幅领先；Windows 大目录仍需实机复测后决定是否默认启用。

## 阶段四：匹配器优化

### 4.1 规则关键词预过滤

- [x] `patterns.Pattern` 增加 `Hints []string`。
- [x] 单行匹配前先检查 hints：
  - 有 hints：命中任意 hint 才运行正则。
  - 无 hints：保持当前行为。
- [x] 大小写不敏感规则预先 lower 文本片段。

建议 hints：

- 私钥：`BEGIN`, `PRIVATE KEY`
- API 密钥：`api`, `key`, `secret`, `token`
- 数据库连接串：`password`, `pwd`, `user`, `server`, `host`
- 密码：`password`, `passwd`, `pwd`, `pass`
- JWT：`eyJ`
- 邮箱：`@`
- URL：`http://`, `https://`
- Windows 路径：`:\`

验收标准：

- 现有模式示例测试不回归。
- 大量普通文本文件 benchmark 中正则调用次数明显下降。

实现补充：已新增 `BenchmarkMatchContentNoHints` / `BenchmarkMatchContentWithHints`，本机验证命令：

```bash
go test -bench=MatchContent -benchmem ./internal/scanner
```

### 4.2 数字类规则候选过滤

- [x] 手机号、身份证、银行卡先做数字候选扫描。
- [x] 候选命中后再运行正则，保持准确性。

验收标准：

- 手机号/身份证/银行卡现有测试不回归。
- 对无数字文本文件，跳过数字类正则。

实现补充：当前采用“全文最少数字数”保守过滤，覆盖手机号、身份证、银行卡和 IP；数字数量不足时才跳过，不依赖上下文关键词，避免漏报。

## 阶段五：单文件规则和白名单配置

坚持“一份 exe，无外部必需配置文件”的前提下，采用浏览器本地配置 + 请求携带方案。

### 5.1 Web localStorage 持久化

- [ ] 规则 CRUD 存在浏览器 `localStorage`。
- [ ] 白名单配置存在浏览器 `localStorage`。
- [ ] 每次扫描时，前端把规则和白名单随 `/api/scan` 请求发送给后端。
- [ ] 后端只为本次扫描临时编译规则，不写磁盘。

验收标准：

- 关闭程序再打开，同一浏览器可保留自定义规则。
- 删除浏览器缓存后恢复内置默认规则。
- exe 旁边不生成任何配置文件。

### 5.2 导入/导出配置包

- [ ] Web 提供“导出配置”按钮，下载 JSON。
- [ ] Web 提供“导入配置”按钮，写入 localStorage。
- [ ] 配置 JSON 包含版本号，便于后续迁移。

验收标准：

- 不要求用户长期维护配置文件。
- 需要迁移时可以手动导入导出。

### 5.3 后端动态规则编译

- [ ] 新增 `patterns.CompileCustom([]Pattern)`。
- [ ] `scanner.Config.Patterns []patterns.Pattern`，为空时使用内置规则。
- [ ] Web `/api/patterns/validate` 校验规则是否合法。

验收标准：

- 非法正则不会导致服务 panic。
- 动态规则只影响本次扫描，不污染全局内置规则。

## 风险和决策

### fastwalk 是否默认启用

先保持实验开关。只有在 Windows 大目录 benchmark 明显优于标准库，且权限、junction、长路径行为稳定后，再默认启用。

### OCR 默认策略

全盘快速模式默认关闭 OCR。OCR 是慢路径，且全盘图片扫描的误报和耗时不可控；需要 OCR 时使用自定义扫描的小范围路径。

### 规则 CRUD 存储位置

不写 `rules.json`，避免破坏“单文件工具”的定位。默认使用 `localStorage`；需要迁移时导入/导出配置包。

### 匹配器重构范围

优先加 hints 预过滤，不急于替换正则库。Go 标准 `regexp` 已是 RE2，当前规则数量下通常不是首要瓶颈。

## 建议执行顺序

1. 阶段一：扫描面收缩。
2. 阶段二：全盘快速模式和跳过统计。
3. 阶段三：fastwalk 实验开关和 benchmark。
4. 阶段四：规则 hints 预过滤。
5. 阶段五：Web localStorage 规则 CRUD 和白名单配置。

每完成一个阶段都需要运行：

```bash
go test ./...
go vet ./...
go test -race ./internal/scanner
go build -tags gui -o /tmp/sensitive-scanner-gui ./cmd/gui
```
