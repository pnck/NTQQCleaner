# CLAUDE.md — ntqq-cleaner

跨平台 QQ 缓存清理工具（Go + Wails v2 + React/TanStack Virtual）。设计文档在 `docs/`（7 份，是需求与规则的唯一权威来源）；安全红线见 `docs/06_safety_redlines.md`。

## 常用命令（Task，https://taskfile.dev）

```sh
task build-cli                    # 纯 CLI 二进制（无需 GUI 依赖，任何环境可构建）
task build-cli[:<os>-<arch>]      # 平台 CLI 二进制（CGO_ENABLED=0 纯 Go 交叉，CI 发布用）
task build[:<os>-<arch>]          # GUI 二进制（go build 层），省略 = dev：开 inspector、不 strip
task build[:<os>-<arch>]:debug    # 显式 debug 模式（与省略默认等价）
task build[:<os>-<arch>]:release  # release 模式（-s -w + trimpath，无 debug 注入）
task dev / bundle / bundle:release  # 热重载 / dev .app（带 inspector）/ release .app
task bundle[:<os>-<arch>]         # 平台 dev 包（省略 = dev，与宿主一致；-debug 开 inspector）
task bundle[:<os>-<arch>]:release # 平台 release 包（CI matrix 单元直接调用）
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
- **GUI 再分 debug / release 两档**（显式模式 token，完全对称；省略时
  build/bundle 全族统一 = dev——release 一律显式 `:release`）：
  - **debug**（`build[:<os>-<arch>]:debug` / `bundle[:<os>-<arch>]:debug`，
    build 族省略 = debug）：`-tags wails,production,debug,devtools` +
    `-gcflags all=-N -l` + 不 strip + `-X main.openInspectorOnStartup=1`。
    wails 的 `debug`/`devtools` 是独立 build tag（`internal/app/app_debug.go`
    / `app_devtools.go`）：devtools 开 inspector——**Windows WebView2 启动
    自动打开 devtools（F12 亦可）**，macOS 经注入变量自动打开 WebKit
    inspector。bundle 层等价地走 wails CLI 的 `-debug` 标志（自动加上述
    标签 + buildvcs=false）
  - **release**（`build[:<os>-<arch>]:release` / `bundle[:<os>-<arch>]:release`）：
    `-tags wails,production` + `-ldflags -s -w` + `-trimpath`，无任何 debug 注入
- `go mod tidy` 默认启用全部标签，wails 依赖会保留在 go.mod
- 逻辑层（internal/*）**不 import Wails**：事件走 `app.Emitter` 接口，CLI 与 GUI 共用同一管线（docs/04 §8）

## 容器环境（本开发容器）

- Go 工具链在 `/opt`（持久化卷）：`source /opt/env.sh`（~/.bashrc 已追加）
- 容器**无显示器、无 C 编译器**：GUI 无法在容器内运行
- **wails v2.14.0**（go.mod）+ go-webview2 v1.0.22；用户 Mac 的 wails CLI 需同步 `go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0`（CLI 与库版本不一致会构建异常；webview2 版本跟库走）
- **Wails v2 对话框（任何版本）只有 Go 侧 API**（注入前端的 runtime.js 不含 dialog 函数）：项目里对话框在 `main_wails.go` 的 `Dialogs` 结构体（window.go.main.Dialogs），前端经 api.ts 调用 —— 这也符合"确认在 Go 侧"的红线
- **Windows MessageDialog 完全忽略自定义按钮文案**（MessageBoxW 无自定义按钮；WarningDialog 恒单 OK 按钮、返回 "Ok"）——现已不用：Windows 的二元确认走平台层 `TaskDialogIndirect`（comctl32，Vista+，现代化样式；纯 syscall 可容器交叉编译），darwin/linux 回退 wails NSAlert/GTK。契约统一 "Yes"/"No"、默认 No（main_wails.go `confirmYesNo`，前端按 "Yes" 判断）
- darwin 构建需要 `CGO_LDFLAGS=-framework UniformTypeIdentifiers`（wails 2.14 的 WailsContext.m 用 UTType 但未链接该 framework）
- 交叉编译（实测 + 官方文档）：
  - **裸 `go build` 必须带 `production` 标签**（wails build 自动加）：缺了会编进 app_default_*.go stub，运行时报 "Wails applications will not build without the correct build tags"
  - **Windows exe**：后端纯 Go（WebView2 经 syscall）→ 容器内可交叉产出**真实可用** exe（`task build:windows-amd64`，dev 模式带 WebView2 devtools；分发用 `task build:windows-amd64:release`）；`-H windowsgui` 已内置（PE GUI 子系统，无启动控制台黑框）
  - **macOS**：WKWebView 后端为 ObjC/cgo → 容器无 C 编译器，编译期即报 `clang not found` → 必须在 macOS 主机或 macos CI 上构建（官方 crossplatform 文档也是每平台原生 runner 构建）
  - **Linux**：需 webkit2gtk（编译+运行），容器两者皆无。ubuntu-24.04 只有
    4.1 dev 包 → 构建必须带 `webkit2_41` tag（wails 按此选 4.1 的
    pkg-config）——已内联进 Taskfile 的 build:p / bundle:p（linux 平台
    自动追加），CI ubuntu runner apt 装 `libwebkit2gtk-4.1-dev` 即可
  - Task CLI 已装在 `/opt/bin/task`、pnpm 在 `/opt/bin/pnpm`（持久化）；构建目标见 `Taskfile.yml`（容器内直接 `task build-cli` / `task verify` 可用）
  - pnpm 配置（版本方言多，勿动）：根 `pnpm-workspace.yaml`（packages + allowBuilds 映射，供根目录 install）；所有 pnpm 命令带 `--config.verify-deps-before-run=warn`（pnpm 11 从 workspace 成员目录跑命令时不读根 workspace 文件的 allowBuilds，会误报 ERR_PNPM_IGNORED_BUILDS；esbuild 经 optionalDependencies 无需 postinstall）
  - pnpm 的 content-addressable store 落在 `/workspace/.pnpm-store`（已 gitignore）
- 出站网络经 proxy 容器透明代理，npm/go 下载可用

## CI（.github/workflows/，两个 workflow 不合并）

- **release.yml**：push tag `vX.Y.Z`（精确语义 tag；workflow_dispatch 带
  tag 输入用于失败后补发）→ 完整门禁（gofmt + test + smoke + vet +
  typecheck）→ 4 平台矩阵仅编译 `bundle:<os>-<arch>:release` → 正式
  release（**永不删除**；补发时已存在则 upload 补传 assets）
- **preview.yml**：push main → 同一完整门禁 → 矩阵编译并发布每平台的
  cli + bundle debug/release（裸 GUI build 层与 bundle 冗余——同一
  二进制少了 manifest/图标/版本资源——不发布，仍在 Taskfile 里供
  容器快速验证；前端不单独发布）→ 发布 prerelease
  `v<base.patch+1>-alpha.N`（base = 最新正式 tag，无 tag 兜底
  v0.1.0 → 首个为 v0.1.1-alpha.1；N 按既有同名 alpha 数从 1 滚动，
  新正式 tag 换前缀即重置）→ **只留最近 10 个** preview，删除时
  `--cleanup-tag` 连带清 alpha tag（alpha tag 由 gh 服务端创建，
  不触发 push 事件，不会误触发 release.yml）。资产命名
  `ntqq-cleaner-<target>-<mode>-<version>[.ext]`，版本后缀在末尾
  （插在扩展名之前；linux ELF 无扩展名直接追加）
- 矩阵 runner（按平台能力分型）：windows-amd64 → `ubuntu-24.04` 交叉
  （后端纯 Go，容器已验证；Taskfile 的 sh: 变量是 unix 方言，windows
  runner 上 task 求值 `go env GOOS` 直接 127，故不建在 windows
  runner）；darwin-arm64 → `macos-15`；darwin-amd64 → `macos-15-intel`
  （macos-13 已退役，x64 专用标签）；linux-amd64 → `ubuntu-24.04`
  （apt 装 webkit2gtk 4.1 dev）。前端只在 gate（ubuntu）构建一次，
  矩阵单元下载 dist、不装 node/pnpm（Taskfile 的 frontend status
  检测到 dist 即跳过）。publish 并发用 job 级 concurrency 串行化（防
  alpha N 撞名）
- 版本计算不用 `git tag -l` 的 glob（fnmatch 的 `[0-9]*` 会连
  `-alpha.7` 一起吞掉）：grep 锚定整串 + `sort -V`
- 矩阵产物是 GitHub Actions artifacts（纯传输，**1 天过期**防 quota
  打满）；发布物以 release assets 形式持久保留
- actions 版本（2026-08 调研）：checkout v7 / setup-go v7 / setup-node
  v7 / pnpm/setup v2（pnpm/action-setup 的继任，需 pnpm ≥11）/
  upload-artifact v7 / download-artifact v8；release 操作用预装的 gh
  CLI（不用 softprops）；task CLI 与 wails CLI 用 `go install` 钉版本
  （v3.52.0 / v2.14.0）装到 `$GITHUB_WORKSPACE/.tools`。CI 装 go.mod
  声明的工具链（1.25.0），并在 setup-go 之后重新导出 GOTOOLCHAIN=auto：
  setup-go 无条件经 GITHUB_ENV 导出 local（会覆盖 job 级 env，实测），
  而 task v3.52.0 需要 go ≥ 1.25.10——auto 让 go install 按需下载更高
  工具链；项目自身已用精确 1.25.0 工具链实测 build+test 通过，声明
  底线成立，不会触发切换
- 平台坑（首跑实测）：① windows 单元不建在 windows runner——Taskfile
  的 sh: 变量（command -v/uname/sed）是 unix 方言，windows runner 上
  task 求值 `go env GOOS` 时反斜杠路径被 shell 转义，直接 127；后端
  纯 Go，ubuntu 交叉产物真实可用（容器已验证）；② Taskfile 的
  bundle:p 用 `hasPrefix "linux" .PLATFORM`（sprig 参数序是
  hasPrefix <前缀> <字符串>，反了会静默求值为 false，webkit2_41
  不追加 → pkg-config 找 4.0 失败）；③ pnpm 只在 gate（ubuntu）用
  一次（package-json-file 指向 frontend/package.json 的 11.21.0）：
  pnpm 11 无 Intel macOS 二进制（Node SEA bug，pnpm#11423），但矩阵
  单元已不需要 pnpm——前端 dist 平台无关，gate 构建一次经 artifact
  下发；④ gh release create 忽略 `file#displayName`（实测 alpha.1
  资产名无版本前缀）——资产改名在 publish 的本地暂存目录完成；
  ⑤ upload-artifact 以给定 path 为根存储（artifact 内无
  frontend/dist 前缀），download 时需显式 `path: frontend/dist`

## 结构（三层变异轴：OS API × QQ 平台 × QQ 版本，全部解耦）

```
main.go / main_wails.go / main_manifest.go   入口：scan/clean/gui 子命令
internal/platform    OS 适配层：Adapter 接口（QQProcesses / DeleteFile /
                     MoveFile / Reveal / FreezeAnimatedThumbs），darwin/
                     windows/linux 各一个 build-tagged 实现。删除/移动语义
                     入本层（POSIX unlink 与 Windows DeleteFile 不同：
                     只读属性/共享模式/跨卷）；平台显示政策也入本层
                     （Windows 专有：动图缩略图首帧静态化）。新增平台 =
                     新增一个适配器文件，上层零改动
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
internal/logring    内存环形日志 + 崩溃转储（debug.SetCrashOutput，panic 时
                     环形缓冲追加进 crash-<ts>.log；崩溃文件 + 逐操作 attempt
                     日志（API 调用前落盘）+ 退出清理（正常退出才删：
                     clean-exit 标记 + 启动清扫）仅 Windows 启用，build tag
                     静态分派——docs/09 §3.5，POSIX 无文件方案）
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
