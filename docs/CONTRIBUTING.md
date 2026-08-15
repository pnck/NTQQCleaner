# 开发者文档（CONTRIBUTING）

> 请把这个文档交给 LLM agent 阅读

## 1. 项目概览

**NTQQ Cleaner** 是跨平台（macOS / Windows）NTQQ 缓存清理工具：
Go + Wails v2（内嵌 Web UI）+ React 19 / TanStack Virtual 虚拟化照片墙，
另提供 CLI 子命令（scan / clean）供脚本与 CI 使用。

- **安全定位**：默认 dry-run，删除需显式确认；全部安全红线在 Go 侧
  强制实现，前端视为不可信输入（docs/06）。
- **数据来源**：对 QQ 客户端存储/清理逻辑的逆向结论（docs/01–03、
  08），工具不做任何加密 DB 解析。
- **核心策略**：可清性 = 白名单结构校验 + 类别门控，无打分模型；
  「选什么清什么」由用户的命名筛选器表达（docs/03 §4）。

## 2. 构建与验证

构建统一走 [Task](https://taskfile.dev)（`Taskfile.yml`）：

```sh
task build-cli              # 纯 CLI 二进制（任何环境可构建）
task build                  # 宿主原生 GUI 二进制（go build 层）
task build:<os>-<arch>      # 交叉二进制：windows-amd64 / darwin-arm64 /
                            # darwin-amd64 / linux-amd64
task dev / bundle           # 热重载 / dev .app 打包（需 wails CLI）
task bundle:release         # 宿主 release 包
task bundle:<os>-<arch>     # 平台 release 包（CI matrix 单元直接调用）
task verify                 # CI 门禁（test + smoke + vet）
task frontend               # 重建 frontend/dist（存在即跳过；源码变更后 --force）
task frontend:typecheck
```

要点（详见 CLAUDE.md 与 Taskfile.yml 注释）：

- `frontend/dist` 不提交 git：由开发容器构建落在挂载盘，宿主机
  `task build` 直接 embed；只有改前端源码后才需要 pnpm。
- 裸 `go build` 必须带 `production` 标签（缺了编入 Wails stub，
  运行时报错）；GUI 构建用 `-tags wails,production`。
- 运行依赖：Windows 需 WebView2（Win10/11 自带）；macOS 用系统
  WKWebView；Linux 需 webkit2gtk（编译 + 运行）。

## 3. 代码地图

架构与数据流细节见 docs/04 §2，这里只给地图：

```
main.go / main_wails.go / main_manifest.go   入口：scan/clean/gui 子命令
internal/platform    OS 适配层（Adapter 接口，darwin/windows/linux
                     build-tagged 各一个）：进程守卫、删除/移动语义
                     （只读属性、跨卷）、Reveal/OpenFile
internal/qq          QQ 知识层：Knowledge 接口 + probe 注册表 +
                     两遍 Detect 分派（capable 优先）+ 根候选汇总
internal/qq/impl/nt      NT 架构：缓存布局跨平台共享；实例/全局目录的
                         OS 差异收敛为 layoutSpec 数据（layout.go +
                         spec_<os>.go）
internal/qq/impl/legacy  旧版 QQ：识别 + 只统计，fail-closed 不扫描不清理
internal/qq/generic.go   兜底：未知布局 fail-closed
internal/qqimpl          副作用导入注册 probe；新增实现在此加一行
internal/discovery   数据根发现、账号组装（NtRel 拼接）、旧版摘要
internal/classify    白名单遍历 + FileEntry + HashDuplicates（size
                     冲突组 → SHA-256，跨账号）
internal/rules       白名单/黑名单政策 + reason 标签 + 配置
internal/report      UI/CLI 共享模型（预览走 /preview/{id}）
internal/clean       删除执行：进程保护→白名单→适配器删除/移动→审计
internal/app         Engine（CLI/GUI 共享管线）+ Backend（GUI 绑定）
internal/testutil    fixtures（docs/05 §6）+ 固定时钟 testutil.Now
frontend/            React 19 + 虚拟化照片墙（页面 ≤200 行）
tests/cli_smoke.sh   CLI 端到端冒烟
```

**三层变异轴**（OS API × QQ 平台 × QQ 版本，全部解耦）：

| 变异 | 抽象 | 新增一种的代价 |
|---|---|---|
| OS API | `platform.Adapter` | 新增一个 build-tagged 适配器文件，上层零改动 |
| QQ 平台×版本族 | `qq.Knowledge` + probe | 新增一个 impl 包 + `qqimpl` 加一行导入 |
| NT 布局的 OS 差异 | nt 包 `layoutSpec` 数据 | 新增/修改一个 spec，linux 上可直接测试 |

平台差异的现状汇总见 docs/08（Windows `<QQ号>/nt_qq/` 布局、旧版兼容
层、`%APPDATA%` 次级回退、官方「保留+统计」对齐）；mac 是主验证平台。

## 4. 文档地图

| 文档 | 定位 |
|---|---|
| `docs/CONTRIBUTING.md` | **本文件**：总纲、上手、流程约定 |
| `docs/README.md` | 设计文档包索引（docs/01–08 的目录与速查） |
| `docs/01_data_structure.md` | 缓存目录结构全解（biz 码表、Emoji 五子类、错拼说明） |
| `docs/02_account_identification.md` | 账号明文识别（mac 三源） |
| `docs/03_clean_rules.md` | 清理规则与可清性政策（官方「缓存文件」category、统计≠清理双口径） |
| `docs/04_tool_design.md` | 架构与实现指导（技术栈论证、模块骨架） |
| `docs/05_sample_data.md` | 开发机样例数据（测试/验证参考，非通用） |
| `docs/06_safety_redlines.md` | 安全红线（硬约束，代码评审第一优先级） |
| `docs/07_ui_design.md` | GUI 框架与性能预算（虚拟化/预览策略） |
| `docs/08_windows_legacy_compat.md` | Windows 布局 + 旧版兼容层设计（含展示映射、待确认清单） |

设计文档是**需求与规则的唯一权威来源**；代码与文档不一致时以文档为准，
并推动文档修订（见 §5 流程）。

## 5. 必须遵守的约定

- **文档先行**：实质功能/架构变更先写 `docs/NN_*.md` 设计文档（含红线
  与测试策略），更新 docs/README 索引，再实现。逆向/实测结论变化时
  同步修订文档。
- **提交**：无 remote，提交直接落 main，不开分支；容器无 gpg，用
  `git -c commit.gpgsign=false commit`。提交末尾 co-author 写
  `Co-Authored-By: Claude Code <noreply@anthropic.com>`（不带模型名）。
- **代码风格**：gofmt（`task fmt`）；前端页面 ≤200 行、PhotoWall
  虚拟化 + 无限分页、行状态由 App 持有；测试时钟用 `testutil.Now`
  （2026-08-10）注入，fixture mtime 固定、断言可复现。
- **展示映射约定**：QQ 目录名存在官方拼写错误（`flashfransfer`、
  `BaseEmojiSyastems`、`EmojiSystermResource`，逆向铁证）——磁盘匹配
  保持原始拼写（fail-closed 结构不能改），界面展示经 `BIZ_LABEL` /
  `SUB_LABEL`（前端）与 `displaySub`（Go 侧 dupLabel）映射为无错别字
  名称；CLI manifest 与表达式规范 token 保留磁盘原名（机器可读真相）。
- **安全红线速览**（完整版 docs/06）：默认 dry-run 零写入；白名单/
  黑名单逐文件二次校验；QQ 运行中拒绝清理（可二次确认覆盖）；删除
  必留痕（结果回显 + 审计按需 opt-in）；前端不可信，红线全在 Go 侧。

## 6. 常见任务手册

| 任务 | 做法 |
|---|---|
| 改前端后重建产物 | `pnpm --config.verify-deps-before-run=warn --dir frontend build`（或 `task frontend --force`），再 `task build:windows-amd64` 等重新 embed |
| 交叉编译 Windows exe | `task build:windows-amd64`（容器内即可，`-H windowsgui` 已内置）；产物 `build/bin/ntqq-cleaner-windows-amd64.exe` 含 CLI 子命令 |
| 新增 OS 平台适配 | 新增 `internal/platform/adapter_<os>.go` 实现 Adapter（进程名/删除移动语义/Reveal/OpenFile），上层零改动 |
| 新增 QQ 平台/版本族 | 新增 `internal/qq/impl/<x>` 实现 Knowledge（capable 才可扫描）+ probe，`internal/qqimpl` 加一行导入；根候选经 `RegisterRoots` |
| 在 linux 上测 Windows 行为 | nt 包测试直接替换 `currentSpec = windowsSpec`（docs/08 §5） |
| 真机验证 | 按 docs/08 §6 待确认清单逐项核对；新平台首跑重点看根探测/实例判定/进程守卫 |
