# 09 — 开发者指南

> 面向开发者的项目入口文档：构建、架构、约定与文档索引。
> 面向用户的应用介绍在仓库根 `README.md`。需求与规则的唯一权威来源
> 是 `docs/01–08`，本文件只是它们的导航与工程说明。

## 1. 项目形态

跨平台（macOS/Windows）NTQQ 缓存清理工具：**Go + Wails v2**（内嵌
Web UI）+ React 19 / TanStack Virtual 虚拟化照片墙，另提供 CLI 子命令
（scan/clean）供脚本/CI。**默认 dry-run，删除需显式确认**，安全红线
全部在 Go 侧（docs/06）。

## 2. 构建（Taskfile，https://taskfile.dev）

```sh
task build-cli         # 纯 CLI 二进制（无需 GUI 依赖，任何环境可构建）
task build             # 当前主机原生 GUI 二进制
task build-windows     # 交叉编译 windows/amd64 exe（任意主机；纯 Go 后端，实测可用）
task build-darwin-*    # macOS GUI —— 只能在 macOS 主机上跑（ObjC/cgo）
task bundle            # macOS .app 打包（需 wails CLI）
task dev               # 热重载（需 wails CLI + pnpm）
task test / smoke      # go test / CLI 端到端冒烟
task frontend          # 重建 frontend/dist（存在即跳过；源码变更后 --force）
pnpm --dir frontend typecheck
```

- `GO`/`PNPM` 变量自动探测 PATH（可 `GO=/opt/go/bin/go task ...` 覆盖）
- **frontend/dist 不提交 git**：由开发容器构建落在挂载盘，宿主机
  `task build` 直接 embed 现成 dist；只有改前端源码后才需要 pnpm
  （`task frontend --force`）
- 构建标签：默认 = CLI-only（`main_wails.go` 被 `//go:build wails`
  排除，`gui_stub.go` 顶替）；`-tags wails` = GUI。裸 `go build` 必须
  带 `production` 标签（缺了会编进 Wails stub，运行时报错）
- Windows exe 需要 WebView2 运行时（Win10/11 自带）；macOS 用系统
  WKWebView；Linux 需 webkit2gtk（编译+运行）

## 3. 架构（三层变异轴，全部解耦）

```
main.go / main_wails.go / main_manifest.go   入口：scan/clean/gui 子命令
internal/platform    OS 适配层：Adapter 接口（QQProcesses / DeleteFile /
                     MoveFile / Reveal / OpenFile），darwin/windows/linux
                     各一个 build-tagged 实现。删除/移动语义入本层
                     （POSIX unlink 与 Windows DeleteFile 不同：只读属性/
                     共享模式/跨卷）。新增平台 = 新增一个适配器文件，
                     上层零改动
internal/qq          QQ 知识层抽象 + 版本 dispatcher：Knowledge 接口 +
                     probe 链注册表（Detect 两遍扫描：capable 优先）。
                     上层（discovery/classify/rules/clean）经窄接口显式
                     接收知识实现
internal/qq/impl/nt      NT 架构实现：缓存布局（Pic/Video/… 的 {YYYY-MM}/
                         Ori/Thumb 结构）跨平台共享；实例目录/全局目录
                         的 OS 差异收敛为 layoutSpec 数据（layout.go +
                         spec_<os>.go），linux 上可换 spec 直接测 Windows
                         行为
internal/qq/impl/legacy  旧版 QQ（数字目录 + msg3.0.db）兼容层：识别 +
                         只统计报告，fail-closed 不扫描不清理（与官方
                         「保留+统计」策略一致）
internal/qq/generic.go   包内兜底：未知布局 fail-closed（拒绝扫描/清理，
                         仅保留版本无关黑名单底线）
internal/qqimpl          副作用导入注册 probe 与根路径；新增实现在此加一行
internal/discovery   数据根发现与账号组装（经 Detect 分派；NtRel 拼接
                     nt_data 路径；旧版摘要 LegacySummary CLI/GUI 共用）
internal/classify    白名单遍历 + FileEntry 组装（Options.K 注入）+
                     二次扫描 HashDuplicates（size 冲突组 → SHA-256）
internal/rules       白名单/黑名单政策 + reason 标签（Reason）+ 配置
internal/report      UI/CLI 共享模型（不暴露绝对路径，预览走 /preview/{id}）
internal/clean       删除执行：进程保护→白名单→适配器删除/移动→审计
internal/app         Engine（Detect 分派 + 共享管线）+ Backend（GUI 绑定）
internal/testutil    fixtures：假 QQ 目录树（docs/05 §6），固定时钟 testutil.Now
frontend/            React 19 + @tanstack/react-virtual 照片墙
tests/cli_smoke.sh   CLI 端到端冒烟
```

数据流：`discover → classify → 内容哈希二次扫描 → reason/关联索引 →
report ──(dry-run)──▶ UI 展示 └─(用户确认)──▶ clean`。

**平台/版本兼容层摘要**（细节见 docs/08）：

| 轴 | macOS | Windows |
|---|---|---|
| 数据根 | 容器路径（逆向复核） | `Documents\Tencent Files`（SHGetKnownFolderPath 定位链）+ `%APPDATA%\Tencent\QQ` 次级回退 |
| 账号实例 | `nt_qq_<32hex>` 目录 | `<QQ号>/` 数字目录（必含 `nt_qq/nt_data`），目录名即 QQ 号 |
| nt_* 位置 | 实例目录下 | `<QQ号>/nt_qq/` 下；实例顶层另有旧库残留（只统计） |
| 全局目录 | `<根>/global/` | `<根>/nt_qq/global/` |

**展示映射约定**：QQ 目录名存在官方拼写错误（`flashfransfer`、
`BaseEmojiSyastems`、`EmojiSystermResource`，逆向铁证）——**磁盘匹配
保持原始拼写**（fail-closed 结构不能改），界面展示经 `BIZ_LABEL` /
`SUB_LABEL`（前端）与 `displaySub`（Go 侧 dupLabel）映射为无错别字
名称；CLI manifest 与表达式规范 token 保留磁盘原名（机器可读真相）。

## 4. 安全红线（docs/06，代码评审第一优先级）

| 红线 | 实现位置 |
|---|---|
| 默认 dry-run，零写入 | `app.Engine` 只读；`clean.Run` 需 Force+Confirmed |
| 白名单/黑名单逐文件二次校验 | `rules.Whitelisted/Blacklisted` + `clean.VerifyPath` |
| QQ 运行中拒绝清理（可二次确认覆盖） | `clean.QQRunning`（开始前 + 每 30s 复查） |
| 删除必留痕：结果回显 + 审计按需（显式 opt-in） | `clean.deleteOne` + 统计/跳过失败明细 |
| 前端不可信，红线全在 Go 侧 | `internal/app` 不暴露任意路径/文件能力 |

## 5. 配置

`config.example.yaml` 含全部默认值（对齐 QQ 官方 3 天规则
`0x3F47F = 259199s`）。工具自身的全部文件都在系统临时目录
（`<tmp>/ntqq-cleaner/`，本次开机内跨启动复用，OS 自动清理）；
GUI 设置对话框可改。

## 6. 测试

```sh
task test    # 单元 + 集成 + 红线测试（fixture 见 docs/05 §6）
task smoke   # CLI 端到端冒烟：scan → manifest → clean（红线验证）
```

红线测试覆盖：dry-run 零写入（fixture mtime 全量比对）、黑名单永不
入扫描结果、`nt_db` 不清理、无 force/确认拒绝、QQ 运行中拒绝。
Windows 布局行为在 linux 上经 layoutSpec 切换直接测试（docs/08 §5）。

## 7. 开发约定

- **提交**：无 remote，提交直接落 main，不开分支/worktree；容器无 gpg，
  用 `git -c commit.gpgsign=false commit`。提交末尾 co-author 用
  `Co-Authored-By: Claude Code <noreply@anthropic.com>`（不带模型名）。
- **文档先行**：实质功能/架构变更先写 `docs/NN_*.md` 设计文档（更新
  docs/README 索引），再实现；文档是需求与规则的唯一权威来源。
- **无打分/分级模型**（已整体移除）：可清性 = 白名单类别门控 +
  结构红线；选择权在用户筛选器。Reason 只是说明标签。
- **去重按真实内容哈希**：扫描后二次 pass（size 冲突组 → SHA-256，
  跨账号）；文件名 md5 只做 Ori/Thumb 配对，绝不再因同名缩略图误标
  重复。
- 前端页面大小 200 行，PhotoWall 虚拟化 + 无限分页；行状态由 App 持有。
- 测试时钟用 `testutil.Now`（2026-08-10）注入；fixture mtime 固定，
  断言可复现。

## 8. 文档索引

| 文档 | 内容 |
|---|---|
| `docs/01_data_structure.md` | 缓存目录结构全解（biz 码表、Emoji 五子类、错拼说明） |
| `docs/02_account_identification.md` | 账号明文识别方案（mac 三源） |
| `docs/03_clean_rules.md` | 清理规则与可清性政策（官方「缓存文件」category、统计≠清理双口径） |
| `docs/04_tool_design.md` | 架构与实现指导 |
| `docs/05_sample_data.md` | 开发机样例（测试验证参考） |
| `docs/06_safety_redlines.md` | 安全红线（硬约束） |
| `docs/07_ui_design.md` | GUI 框架与性能预算 |
| `docs/08_windows_legacy_compat.md` | Windows 布局 + 旧版兼容层设计（含展示映射、待确认清单） |
| `docs/09_developer_guide.md` | 本文件：构建/架构/约定/索引 |
