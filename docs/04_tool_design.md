# 04 — 工具架构与实现指导（Go + Wails 内嵌 Web UI）

> 本文件是编码的**直接参考**：技术栈、模块划分、前端交互协议、每个模块的实现要点与代码骨架。
> 目标形态：**跨平台的通用缓存清理工具**，任何人下载单二进制即可用，默认 dry-run。

---

## 1. 技术栈决策（已调研定案）

| 项 | 选择 | 理由 |
|---|---|---|
| 语言 | **Go**（≥ 1.21） | 单二进制、静态编译；goroutine 高并发、原生 map、crypto/md5（SIMD）；**本工具性能瓶颈是磁盘 I/O，Go 完全够用** |
| GUI | **Wails v2**（`github.com/wailsapp/wails/v2`） | **Go 系预实现度最高的 GUI**：原生对话框/菜单/事件系统（`runtime.EventsEmit`）/自动 Go↔TS 生成/Vite 开发模式/打包链；Windows 无 CGO |
| 前端 | Wails 模板（Vanilla/React/Vue/Svelte），`embed.FS` 打包进二进制 | 模板开箱即用；选 Vanilla 可免 Node 构建链 |
| 配置 | 标准库 `flag` + 可选 YAML（`gopkg.in/yaml.v3`） | 依赖可控 |

**选型过程**（2026-08 对比调研）：
- ❌ `webview/webview_go`：轻但**无预实现 GUI**（无对话框/菜单/事件），需从零搭——不满足"加速开发"
- ❌ 纯 `net/http` + 浏览器：零 CGO 但有 CSRF 风险 + 标签页体验
- ⚠️ Rust + Tauri / egui：性能极致但对本工具**性能过剩**（I/O 瓶颈），Rust 开发成本高
- ⚠️ Go + Fyne：纯原生组件、无 WebView——**备选**（若不要 web 风格）
- ✅ **Wails v2**：预实现 GUI 最全 + Go 并发/性能完全满足 → 定案

**备选**（若不要 web 风格/WebView）：**Go + Fyne**（`fyne.io/fyne/v2`，28k★，表格/表单/对话框原生组件，单二进制无 WebView）。

---

## 2. 总体架构

```
qq-cleaner/
├── main.go                 # 入口：wails.Run + Bind + embed.FS
├── internal/
│   ├── discovery/          # 数据目录发现 + 账号识别（见 02）
│   ├── classify/           # 遍历 nt_data，文件分类（类型/月份/命名解析）
│   ├── rules/              # 白名单/黑名单判定 + reason 标签（见 03）
│   ├── report/             # 统计与报告模型（供 UI 渲染）
│   ├── clean/              # 执行删除（进程保护/备份/二次确认/日志）
│   └── app/                # Backend（Bind 到前端 window.go.backend）
├── frontend/               # Wails 前端（Vue/React/Vanilla，Vite 构建，embed.FS）
├── go.mod
└── Makefile                # wails build → 单二进制
```

**数据流**：
```
discover() → classify() → 内容哈希二次扫描 → reason/关联索引 → report() ──(dry-run)──▶ UI 展示
                                                              └─(用户确认)──▶ clean()
```

> 无打分/分级环节：可清性 = clean 层的白名单/黑名单逐文件重验（docs/03 §4）。
> 内容哈希二次扫描只读不写：仅对大小冲突组的文件计算 SHA-256（docs/03 §4）。

**关键原则**：UI 层只负责展示与触发；**所有安全红线（白名单、进程保护、备份、确认）在 Go 侧强制实现**，UI 视为不可信输入。

---

## 3. 前端交互协议（Wails Bind + Events）

```go
// internal/app/backend.go —— 在 main.go 中 wails.Bind(b) 暴露，前端以 window.go.backend.Xxx() 调用
type Backend struct {
    mu      sync.Mutex
    scanning bool
    results []report.AccountReport
}

type ScanOptions struct {
    Root      string   `json:"root"`      // 空 = 自动探测
    Account   string   `json:"account"`   // 空 = 全部
    MinAgeDays int     `json:"minAgeDays"`// 默认 3（对齐 QQ 3 天基线）
    MinSize   int64    `json:"minSize"`   // 默认 0
    OnlyTypes []string `json:"onlyTypes"` // 空 = 全部
}

type AccountReport struct {
    Hash     string               `json:"hash"`
    QQNum    string               `json:"qqNum"`    // 识别出的 QQ 号；未知为 ""
    NtData   string               `json:"ntData"`   // 绝对路径（展示用）
    TotalFiles int                `json:"totalFiles"`
    TotalSize  int64              `json:"totalSize"`
}

// 方法（前端 window.go.backend 异步调用）
func (b *Backend) Scan(opts ScanOptions) ([]AccountReport, error) // dry-run 扫描
func (b *Backend) Clean(req CleanRequest) (CleanResult, error)    // 真正清理
func (b *Backend) GetConfig() Config
func (b *Backend) SetConfig(c Config) error
func (b *Backend) DiscoverRoots() ([]string, error)
```

**进度推送**（Wails 内置事件系统，比手写回调更简洁）：
```go
// Go 侧：扫描 goroutine 里定期推送
runtime.EventsEmit(ctx, "scan:progress", map[string]any{"done": n, "total": total, "stage": "pic"})
// 前端：
//   wails.Events.On("scan:progress", (p) => {...})
```
- 长任务（扫描 14 万文件）必须在**独立 goroutine** 跑（`go func`），避免阻塞 UI
- 进度节流：每 ≥100ms 或每 1000 文件 emit 一次，避免前端卡顿
- 原生对话框（删除确认/目录选择）直接用 `runtime.MessageDialog` / `runtime.OpenDirectoryDialog`

**筛选表达式管道**（`Filter`，前后端一致）：

```
条件布尔树（AND/OR 组 + 条件叶子，支持括号/引号）
  | select(ori|thumb|dup)    关联展开：把结果集替换为其中文件关联的其它文件
  | order(field, asc|desc)   排序（可叠加 = 多关键字）
  | drop(n)                  跳过前 n 条
  | take(n)                  取前 n 条
```

- **按书写顺序从左到右组合**（函数式）：每个 stage 作用于前一 stage 的
  输出——`take(10) | select(dup)`（先取 10 条再展开）与
  `select(dup) | take(10)`（先展开再截断）语义不同，序列化不做改写；
  管道函数接在表达式末尾，纯管道表达式（无条件）也合法：
  `select(ori, thumb) | take(100)`
- 条件叶子：`字段 操作符 值`，AND（且）/ OR（或）连接，( ) 嵌套分组，含空格
  的值加引号 `"…"`
- 操作符：`=` `!=` `~`（包含 = 子串匹配 LIKE %值%） `in` `>` `>=` `<` `<=`，
  `after`/`before` 是 `>`/`<` 的可读别名（age/month 等时间性字段的自然
  写法，解析后规范化为 `>`/`<`）
  - `in` 的列表必须写在括号内：`biz in (pic, video)`（单值 `biz in pic` 兼容）；
    逗号**只能**出现在 `in(...)` / `select(...)` / `order(...)` 的括号内，不能
    并列语句（`biz in pic, size>0` 是非法语法，报错指向明确）
  - size/age 按数值比较（`in` 支持括号列表）；month 的操作数按**可计算时间**
    比较——YYYY-MM 解析为当月起始时间戳（实现细节不对用户暴露，用户仍写
    YYYY-MM），字符串序在跨年/缺位写法下不可靠；`in`/`after`/`before` 同样适用
  - `md5` / `contentHash` 是普通字符串字段：`contentHash ~ <前缀>` 即可按
    哈希前缀/片段筛选（哈希在详情面板完整显示，可复制）
- `select` 的三个维度**正交**，可多选取并集：`select(ori, thumb, dup)`：
  - `ori`：缩略图 → 其原文件（文件名 md5 配对）；原文件保留自身；无配对无贡献
  - `thumb`：原文件 → 其全部缩略图（多尺寸）；缩略图保留自身
  - `dup`：展开为内容哈希组（字节级相同的全部文件，含列表内自身）；无哈希（大小唯一）无贡献
- 典型用法：`thumb = true AND age >= 90 | select(ori) | take(100)`
  （把 90 天前的缩略图圈定，展开为其原图再看前 100 个）；
  `select(ori, thumb)` = 圈定文件的完整家族（原图 + 全部缩略图）

---

## 4. 模块实现要点（Go 骨架）

### 4.1 discovery — 目录发现与账号识别

```go
package discovery

type Account struct {
    Hash       string // nt_qq_ 后的 32 hex
    QQNum      string // 识别结果，"" 表示未知
    NtData     string
    NtTemp     string
    NtDb       string // 仅供识别，工具不读不写
    LatestMonth string // Pic 最新月目录（新旧排序用）
}

// RootCandidates 按平台返回默认数据根候选（macOS/Windows/Linux）
func RootCandidates() []string {
    // macOS:  ~/Library/Containers/com.tencent.qq/Data/Library/Application Support/QQ
    // Windows: %APPDATA%\Tencent\QQ\...
    // 由用户 --root / UI 覆盖；遍历候选，取第一个存在的
}

func Discover(root string) ([]Account, error) {
    // 遍历 root 下 nt_qq_* 目录 → IdentifyAccount(root, hash, ntData)（见 02）
    // 按 LatestMonth 倒序（最近使用在前）
}
```
- 账号识别三源实现直接复用 `02_account_identification.md` 的逻辑（纯标准库可移植）
- **只读** mmkv/UnitedConfig/Login，绝不修改

### 4.2 classify — 遍历与分类

```go
package classify

type FileEntry struct {
    Path     string `json:"path"`
    Biz      string `json:"biz"`      // pic/video/ptt/emoji/file/dataline
    Sub      string `json:"sub"`      // Ori/Thumb/OriTemp/ThumbTemp/...
    Month    string `json:"month"`    // YYYY-MM 或 ""
    Size     int64  `json:"size"`
    MTime    int64  `json:"mtime"`
    MD5      string `json:"md5"`      // 从文件名提取（32 hex）
    ContentHash string `json:"contentHash"` // SHA-256（二次扫描，大小唯一时为空）
    IsThumb  bool   `json:"isThumb"`
    IsTemp   bool   `json:"isTemp"`
}

// Scan 遍历白名单目录（BizDirs 映射见 01 §2.3），并发可调（worker pool）
func Scan(ntData string, onlyTypes []string, skipDirs map[string]bool, minSize int64) ([]FileEntry, error)

// 二次扫描：size 冲突组 → SHA-256（docs/03 §4）
func HashDuplicates(ctx context.Context, entries []*FileEntry, progress func(done, total uint64)) error
```
- **并发**：目录并行遍历（`errgroup`），单目录内 `filepath.WalkDir`（用 `DirEntry` 避免额外 stat）
- 文件名解析：`^([0-9a-f]{32})(?:_(\d+))?\.(\w+)$`
- 月目录：路径第一段匹配 `^\d{4}-\d{2}$`
- **性能参考**：单账号 14 万文件（Pic），Go 全量扫描应在数秒~十几秒内完成
- **内容哈希二次扫描**：`HashDuplicates` 按字节数分组，只读大小冲突组的文件；
  8 路并发（磁盘 I/O 主导），进度事件 stage="hash"，ctx 可取消。
  实际读取量远小于总量（大小唯一即跳过）；SHA-256 约 1~2GB/s

**维度指引（现象→模型）**：文件分类的轴应包含 `biz`（所属业务）与 `sub`（Ori/Thumb/Temp）两个**平级**维度；
索引应能按"是否 Thumb"独立聚合/筛选（现象：Thumb 可重建、占缓存大头、跨 biz 普遍存在）——具体接口设计由开发者定。

### 4.3 rules — 白名单/黑名单判定 + reason 标签

```go
package rules

type Config struct {
    CleanTemp, CleanThumb    bool             // true/true
    CleanOri, CleanBaseEmoji, CleanMarketface, CleanPersonalEmoji, CleanFile bool
    CleanLog, CleanAvatar    bool
    MinFileSizeBytes         int64
    SkipDirs                 []string
}

// Reason 返回短标签（展示说明，不影响可清性），如「缩略图」「重复出现」
func Reason(f classify.FileEntry, idx MD5Index) string

// Whitelisted：类别门控 + 结构事实（相对 nt_data 的路径）
func Whitelisted(k Knowledge, rel string, cfg Config) bool

// Blacklisted：硬黑名单（状态目录/db 后缀），任何模式不可触碰
func Blacklisted(k Knowledge, abs string) bool
```
- 白名单判定：路径必须落在 01 §2.1 富媒体目录；`SkipDirs` 在 classify 层与 clean 层**双重**过滤

### 4.4 clean — 执行删除（严格安全）

```go
package clean

type Request struct {
    Files     []classify.FileEntry // 用户勾选的文件；逐条重验后才可能删
    BackupDir string               // 非空 = 移动备份；空 = 记录清单
    Force     bool                 // 必须 true（由 UI 二次确认后传入）
}

func Run(req Request) (Result, error) {
    if qqRunning() { return errQQRunning }        // pgrep -f "QQ.app" / tasklist
    if !req.Force { return errNotForced }
    for _, f := range req.Files {
        if !whitelisted(f.Path) { logAndSkip(f); continue } // 白名单二次校验
        if req.BackupDir != "" { os.Rename(f.Path, backup) } else { os.Remove(f.Path) }
        auditLog(f)   // 时间/路径/大小/reason 标签
    }
}
```
- **备份优先**：`--backup-dir` 移动（可恢复）；否则审计日志 + SHA-256
- 每次删除前**再次校验白名单**（防路径穿越/误删）
- 删除逐条执行并记录，失败不中断（继续 + 汇总）

### 4.5 app — Backend（Bind 层，见 §3）

- 方法全部**返回错误而非 panic**；JS 侧统一错误提示
- 扫描/清理用 `sync.Mutex` 防止并发重入（UI 误双击）
- 敏感操作（Clean）在 Go 侧再次执行：进程保护 → 白名单 → 二次确认标志

---

## 5. 安全设计（红线在 Go 侧，UI 不可信）

1. **默认 dry-run**：`Scan` 只读不写；`Clean` 必须显式调用且满足全部前置条件
2. **黑名单双过滤**：classify 层跳过 + clean 层逐文件校验（`06_safety_redlines.md`）
3. **进程保护**：clean 执行前 `pgrep`/`tasklist` 检查 QQ，运行中拒绝
4. **Wails 安全**：仅加载 `embed.FS` 本地资源，禁止远程内容；`Bind` 只暴露白名单方法；
   前端事件与命令不经网络（Wails 原生 IPC，天然无 CSRF）
5. **路径安全**：所有 `filepath` 操作用 `filepath.Clean` + 前缀校验，防 `..` 穿越
6. **审计日志**：每次 Clean 写 `~/.qq-cleaner/audit.log`（或用户指定），含删除清单与校验和

---

## 6. 构建与发布

```makefile
# Makefile（Wails 官方 CLI 打包）
# 开发:  wails dev      （热重载 + Vite）
# 发布:  wails build -clean -trimpath -ldflags="-s -w"
#        → build/bin/qq-cleaner（单二进制，前端已 embed）
# Windows: 需 WebView2 运行时（Win10/11 自带）；wails build 默认无控制台窗口
# macOS:  使用系统 WKWebView，无额外运行时
# Linux:  需 webkit2gtk-4.0 库
```
- 发布：Wails 单二进制 + 可选图标/安装包（`wails build` 支持 macOS .app / Windows .exe）
- 版本：`-ldflags "-X main.version=..."`

---

## 7. 测试建议

1. **单元测试**（纯 Go，`go test`）：
   - discovery（临时目录模拟 `nt_qq_<hash>` + 假 mmkv 内容）
   - rules.Reason（各类别 × 关联组合的标签）/ Whitelisted / Blacklisted
   - classify 文件名/月目录解析
2. **集成测试**：`tests/fixtures/` 构造小型假 QQ 目录树（见 05 §6），跑 Scan 校验报告
3. **UI 测试**：Wails 窗口手测（扫描/勾选/确认/清理流程）；可用 `--headless-scan` 子命令
   让 CI 无需窗口也能跑扫描逻辑
4. **红线测试**：dry-run 零写入（fixtures 目录 mtime 不变）、黑名单目录永不出现、
   `nt_db` 不被清理、QQ 运行中 Clean 被拒

---

## 8. 可选 CLI 子命令（无窗口模式）

`qq-cleaner` 同时提供无 UI 入口（便于脚本/CI/无图形环境）：
```
qq-cleaner scan [--root R] [--account HASH] [--json]     # dry-run 扫描输出 JSON
qq-cleaner clean --file manifest.json [--backup-dir D]    # 按清单执行（需 --force 确认）
qq-cleaner gui                                            # 默认：启动内嵌 Web 窗口
```
- `gui` 为默认命令；`scan/clean` 供无头环境与自动化
- 前端 UI 与 CLI 共享同一 `internal/` 逻辑层，红线一致
