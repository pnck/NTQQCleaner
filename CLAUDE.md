# CLAUDE.md — ntqq-cleaner

跨平台 QQ 缓存清理工具（Go + Wails v2 + React/TanStack Virtual）。设计文档在 `docs/`（7 份，是需求与规则的唯一权威来源）；安全红线见 `docs/06_safety_redlines.md`。

## 常用命令（Task，https://taskfile.dev）

```sh
task build-cli                    # 纯 CLI 二进制（无需 GUI 依赖，任何环境可构建）
task build[:<os>-<arch>]          # GUI 二进制（go build 层），省略 = dev：开 inspector、不 strip
task build[:<os>-<arch>]:debug    # 显式 debug 模式（与省略默认等价）
task build[:<os>-<arch>]:release  # release 模式（-s -w + trimpath，无 debug 注入）
task dev / bundle / bundle:release  # 热重载 / dev .app（带 inspector）/ release .app
task bundle[:<os>-<arch>]:debug   # 平台 dev 包（-debug：开 inspector、不 strip）
task bundle:<os>-<arch>           # 平台 release 包（CI matrix 单元直接调用；= :release）
task verify                       # CI 门禁（test + smoke + vet）
task frontend                     # 重建 frontend/dist（存在即跳过；源码变更后 --force）
task frontend:typecheck
```

- `GO`/`PNPM` 变量自动探测 PATH（可 `GO=/opt/go/bin/go task ...` 覆盖）
- 各目标的环境变量（CGO_ENABLED/GOOS/GOARCH）内联在 task 的 `env:` 里
- **frontend/dist 不提交 git**（.gitignore 内）：由容器构建，落在 /workspace 挂载盘上，宿主机 `task build` 直接 embed 现成 dist；新克隆/无 dist 的环境才需要 pnpm（task frontend --force）

## 构建标签方案

- 默认构建 = **CLI-only**（`main_wails.go` 被 `//go:build wails` 排除，`gui_stub.go` 顶替）
- `-tags wails` = GUI（Wails + embed.FS 前端）
- **GUI 再分 debug / release 两档**（显式模式 token，完全对称；省略时 build 族
  默认 dev，bundle 族宿主默认 dev、平台变体默认 release——兼容 CI 矩阵）：
  - **debug**（`build[:<os>-<arch>]:debug` / `bundle[:<os>-<arch>]:debug`，
    build 族省略 = debug）：`-tags wails,production,debug,devtools` +
    `-gcflags all=-N -l` + 不 strip + `-X main.openInspectorOnStartup=1`。
    wails 的 `debug`/`devtools` 是独立 build tag（`internal/app/app_debug.go`
    / `app_devtools.go`）：devtools 开 inspector——**Windows WebView2 启动
    自动打开 devtools（F12 亦可）**，macOS 经注入变量自动打开 WebKit
    inspector。bundle 层等价地走 wails CLI 的 `-debug` 标志（自动加上述
    标签 + buildvcs=false）
  - **release**（`build[:<os>-<arch>]:release` / `bundle[:<os>-<arch>]:release`，
    bundle 平台变体省略 = release）：`-tags wails,production` +
    `-ldflags -s -w` + `-trimpath`，无任何 debug 注入
- `go mod tidy` 默认启用全部标签，wails 依赖会保留在 go.mod
- 逻辑层（internal/*）**不 import Wails**：事件走 `app.Emitter` 接口，CLI 与 GUI 共用同一管线（docs/04 §8）

## 容器环境（本开发容器）

- Go 工具链在 `/opt`（持久化卷）：`source /opt/env.sh`（~/.bashrc 已追加）
- 容器**无显示器、无 C 编译器**：GUI 无法在容器内运行
- **wails v2.14.0**（go.mod）+ go-webview2 v1.0.22；用户 Mac 的 wails CLI 需同步 `go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0`（CLI 与库版本不一致会构建异常；webview2 版本跟库走）
- **Wails v2 对话框（任何版本）只有 Go 侧 API**（注入前端的 runtime.js 不含 dialog 函数）：项目里对话框在 `main_wails.go` 的 `Dialogs` 结构体（window.go.main.Dialogs），前端经 api.ts 调用 —— 这也符合"确认在 Go 侧"的红线
- **Windows MessageDialog 完全忽略自定义按钮文案**（MessageBoxW 无自定义按钮；WarningDialog 恒单 OK 按钮、返回 "Ok"）：二元确认统一走 `QuestionDialog` YES/NO（MB_YESNO / NSAlert），返回契约 "Yes"/"No"（main_wails.go `confirmYesNo`，前端按 "Yes" 判断）
- darwin 构建需要 `CGO_LDFLAGS=-framework UniformTypeIdentifiers`（wails 2.14 的 WailsContext.m 用 UTType 但未链接该 framework）
- 交叉编译（实测 + 官方文档）：
  - **裸 `go build` 必须带 `production` 标签**（wails build 自动加）：缺了会编进 app_default_*.go stub，运行时报 "Wails applications will not build without the correct build tags"
  - **Windows exe**：后端纯 Go（WebView2 经 syscall）→ 容器内可交叉产出**真实可用** exe（`task build:windows-amd64`，dev 模式带 WebView2 devtools；分发用 `task build:windows-amd64:release`）；`-H windowsgui` 已内置（PE GUI 子系统，无启动控制台黑框）
  - **macOS**：WKWebView 后端为 ObjC/cgo → 容器无 C 编译器，编译期即报 `clang not found` → 必须在 macOS 主机或 macos CI 上构建（官方 crossplatform 文档也是每平台原生 runner 构建）
  - **Linux**：需 webkit2gtk（编译+运行），容器两者皆无
  - Task CLI 已装在 `/opt/bin/task`、pnpm 在 `/opt/bin/pnpm`（持久化）；构建目标见 `Taskfile.yml`（容器内直接 `task build-cli` / `task verify` 可用）
  - pnpm 配置（版本方言多，勿动）：根 `pnpm-workspace.yaml`（packages + allowBuilds 映射，供根目录 install）；所有 pnpm 命令带 `--config.verify-deps-before-run=warn`（pnpm 11 从 workspace 成员目录跑命令时不读根 workspace 文件的 allowBuilds，会误报 ERR_PNPM_IGNORED_BUILDS；esbuild 经 optionalDependencies 无需 postinstall）
  - pnpm 的 content-addressable store 落在 `/workspace/.pnpm-store`（已 gitignore）
- 出站网络经 proxy 容器透明代理，npm/go 下载可用

## 结构（三层变异轴：OS API × QQ 平台 × QQ 版本，全部解耦）

```
main.go / main_wails.go / main_manifest.go   入口：scan/clean/gui 子命令
internal/platform    OS 适配层：Adapter 接口（QQProcesses / DeleteFile /
                     MoveFile / Reveal），darwin/windows/linux 各一个
                     build-tagged 实现。删除/移动语义入本层（POSIX unlink
                     与 Windows DeleteFile 不同：只读属性/共享模式/跨卷）。
                     新增平台 = 新增一个适配器文件，上层零改动
internal/qq          QQ 知识层抽象 + 版本 dispatcher：
                     Knowledge 接口 + probe 链注册表（Detect 从磁盘布局
                     识别平台×版本族）。上层（discovery/classify/rules/
                     clean）通过窄接口显式接收知识实现
internal/qq/impl/nt      NT 架构实现（nt_qq_<32hex> 布局，macOS 已逆向；
                         布局跨平台共享，根路径 per-OS 文件）。仅支持 NT：
                         旧版（数字目录+msg3.0.db）无实现、fail-closed
internal/qqimpl          副作用导入注册 probe 与根路径；新增实现在此加一行
internal/qq/generic.go   包内兜底：未知布局 fail-closed（拒绝扫描/清理，
                         仅保留版本无关黑名单底线）
internal/discovery  数据根发现与账号组装（经 Detect 分派）
internal/classify   白名单遍历 + FileEntry 组装（Options.K 注入）+
                     二次扫描 HashDuplicates（size 冲突组 → SHA-256）
internal/rules      白名单/黑名单政策 + reason 标签（Reason） + 配置
internal/report     UI/CLI 共享模型（不暴露绝对路径，预览走 /preview/{id}）
internal/clean      删除执行：进程保护→白名单→适配器删除/移动→审计
internal/app        Engine（Detect 分派 + 共享管线）+ Backend（GUI 绑定）
internal/testutil   fixtures：假 QQ 目录树（docs/05 §6），固定时钟 testutil.Now
frontend/           React 19 + @tanstack/react-virtual 照片墙
tests/cli_smoke.sh CLI 端到端冒烟
```

## 关键约定

- **安全红线在 Go 侧**，前端不可信（docs/06 §5b）：删除前逐文件重验白名单/黑名单；dry-run 零写入；无 --force + 确认则不可删
- 产品决策（已同步进 docs/06）：① QQ 运行守卫默认拒绝，但经二次确认可带 `IgnoreRunning` 覆盖（POSIX 下 unlink 不被写锁阻塞，残余风险仅是缓存条目失效可重下）；② 审计与移动均为**显式 opt-in**（默认直接删除、不生成审计——十万级文件的逐文件清单过长）：GUI 确认对话框勾选「生成审计记录」/「以移动代替删除」（未设备份目录则拒绝并引导去设置），CLI 对应 `--audit-log`/`--backup-dir`；审计不记 SHA-256 只记路径/大小/时间
- **无打分/分级模型**（已整体移除）：可清性 = 白名单类别门控（config clean_*，GUI 全部放开）+ 结构红线；选择权在用户筛选器。Reason 只是说明标签，不决定可清性
- **去重按真实内容哈希**：扫描后二次 pass（size 冲突组 → SHA-256，跨账号）；「重复出现」reason 与去重建议都以此分组；文件名 md5 只做 Ori/Thumb 配对（原图仍在/有缩略图），绝不再因同名缩略图误标重复
- 测试时钟用 `testutil.Now`（2026-08-10）注入；fixture mtime 固定，断言可复现
- 前端页面大小 200 行，PhotoWall 虚拟化 + 无限分页；行状态（rows/checked）由 App 持有
