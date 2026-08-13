# CLAUDE.md — qq-cleaner

跨平台 QQ 缓存清理工具（Go + Wails v2 + React/TanStack Virtual）。设计文档在 `docs/`（7 份，是需求与规则的唯一权威来源）；安全红线见 `docs/06_safety_redlines.md`。

## 常用命令（Task，https://taskfile.dev）

```sh
task build-cli         # 纯 CLI 二进制（无需 GUI 依赖，任何环境可构建）
task build             # 当前主机原生 GUI 二进制
task build-windows     # 交叉编译 windows/amd64 exe（任意主机）
task build-darwin-*    # macOS GUI —— 只能在 macOS 主机上跑
task dev / bundle      # 需 wails CLI（热重载 / .app 打包）
task test / smoke      # go test / CLI 端到端冒烟
task frontend          # 重建 frontend/dist（存在即跳过；源码变更后 --force）
pnpm --dir frontend typecheck
```

- `GO`/`PNPM` 变量自动探测 PATH（可 `GO=/opt/go/bin/go task ...` 覆盖）
- 各目标的环境变量（CGO_ENABLED/GOOS/GOARCH）内联在 task 的 `env:` 里
- **frontend/dist 不提交 git**（.gitignore 内）：由容器构建，落在 /workspace 挂载盘上，宿主机 `task build` 直接 embed 现成 dist；新克隆/无 dist 的环境才需要 pnpm（task frontend --force）

## 构建标签方案

- 默认构建 = **CLI-only**（`main_wails.go` 被 `//go:build wails` 排除，`gui_stub.go` 顶替）
- `-tags wails` = GUI（Wails + embed.FS 前端）
- `go mod tidy` 默认启用全部标签，wails 依赖会保留在 go.mod
- 逻辑层（internal/*）**不 import Wails**：事件走 `app.Emitter` 接口，CLI 与 GUI 共用同一管线（docs/04 §8）

## 容器环境（本开发容器）

- Go 工具链在 `/opt`（持久化卷）：`source /opt/env.sh`（~/.bashrc 已追加）
- 容器**无显示器、无 C 编译器**：GUI 无法在容器内运行
- **wails v2.14.0**（go.mod）+ go-webview2 v1.0.22；用户 Mac 的 wails CLI 需同步 `go install github.com/wailsapp/wails/v2/cmd/wails@v2.14.0`（CLI 与库版本不一致会构建异常；webview2 版本跟库走）
- **Wails v2 对话框（任何版本）只有 Go 侧 API**（注入前端的 runtime.js 不含 dialog 函数）：项目里对话框在 `main_wails.go` 的 `Dialogs` 结构体（window.go.main.Dialogs），前端经 api.ts 调用 —— 这也符合"确认在 Go 侧"的红线
- darwin 构建需要 `CGO_LDFLAGS=-framework UniformTypeIdentifiers`（wails 2.14 的 WailsContext.m 用 UTType 但未链接该 framework）
- 交叉编译（实测 + 官方文档）：
  - **裸 `go build` 必须带 `production` 标签**（wails build 自动加）：缺了会编进 app_default_*.go stub，运行时报 "Wails applications will not build without the correct build tags"
  - **Windows exe**：后端纯 Go（WebView2 经 syscall）→ 容器内可交叉产出**真实可用** exe（`make build-windows`）
  - **macOS**：WKWebView 后端为 ObjC/cgo → 容器无 C 编译器，编译期即报 `clang not found` → 必须在 macOS 主机或 macos CI 上构建（官方 crossplatform 文档也是每平台原生 runner 构建）
  - **Linux**：需 webkit2gtk（编译+运行），容器两者皆无
  - Task CLI 已装在 `/opt/bin/task`、pnpm 在 `/opt/bin/pnpm`（持久化）；构建目标见 `Taskfile.yml`（容器内直接 `task build-cli` / `task test` 可用）
  - pnpm 配置（版本方言多，勿动）：根 `pnpm-workspace.yaml`（packages + allowBuilds 映射，供根目录 install）；所有 pnpm 命令带 `--config.verify-deps-before-run=warn`（pnpm 11 从 workspace 成员目录跑命令时不读根 workspace 文件的 allowBuilds，会误报 ERR_PNPM_IGNORED_BUILDS；esbuild 经 optionalDependencies 无需 postinstall）
  - pnpm 的 content-addressable store 落在 `/workspace/.pnpm-store`（已 gitignore）
- 出站网络经 proxy 容器透明代理，npm/go 下载可用

## 结构

```
main.go / main_wails.go / main_manifest.go   入口：scan/clean/gui 子命令
internal/discovery  数据根发现 + 三源账号识别（docs/02）
internal/classify   白名单遍历 + 文件分类（biz/sub/category/month/md5）
internal/rules      价值打分（Score/Tier）、配置、白名单/黑名单（docs/03, 06）
internal/report     UI/CLI 共享模型（不暴露绝对路径，预览走 /preview/{id}）
internal/clean      删除执行：进程保护→白名单→备份/SHA-256→审计（docs/06）
internal/app        Engine（CLI/GUI 共享管线）+ Backend（GUI 绑定层）
internal/testutil   fixtures：假 QQ 目录树（docs/05 §6），固定时钟 testutil.Now
frontend/           React 19 + @tanstack/react-virtual 照片墙
tests/cli_smoke.sh CLI 端到端冒烟
```

## 关键约定

- **安全红线在 Go 侧**，前端不可信（docs/06 §5b）：删除前逐文件重验白名单/黑名单；dry-run 零写入；无 --force + 确认则不可删
- 产品决策（偏离 docs/06）：① QQ 运行守卫默认拒绝，但经二次确认可带 `IgnoreRunning` 覆盖（POSIX 下 unlink 不被写锁阻塞，残余风险仅是缓存条目失效可重下）；② 无备份目录时审计只记路径不再算 SHA-256
- 打分模型：score = type(0-40) + time(0-30) + redundancy(0-20) + size(0-10)；tier：safe/suggest/caution/keep；🟠 需 aggressive
- 测试时钟用 `testutil.Now`（2026-08-10）注入；fixture mtime 固定，断言可复现
- 前端页面大小 200 行，PhotoWall 虚拟化 + 无限分页；行状态（rows/checked）由 App 持有
