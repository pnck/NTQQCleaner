# NTQQ Cleaner — 更安全的 NTQQ 缓存清理工具

跨平台（macOS/Windows）QQ 桌面版缓存清理工具：**Go + Wails v2**（内嵌 Web UI）+ React 虚拟滚动照片墙，另提供 CLI 子命令供脚本/CI。**默认 dry-run，删除需显式确认。**

设计依据全部来自 `docs/`（对 QQ 客户端存储/清理逻辑的逆向结论），文档是需求与规则的唯一权威来源。

## 为什么值得做

- QQ 缓存可达数十 GB（图片/视频/表情长期累积），自带清理一刀切 3 天
- 缓存大头是**缩略图**（Thumb 可占 Pic 的 66%），且**可从原图重建 → 删除零数据损失**
- 本工具按「业务类型 × 时间 × 命名筛选器」组织扫描结果，用户按自己的记忆做最终判断

## 功能线（docs/README §1）

1. **缩略图删减**（零损失清理，跨 biz 统一评估）
2. **按业务类型清理**（Pic/Video/Ptt/Emoji/File/dataline，Emoji 分五细类）
3. **命名筛选器**（条件 + 排序 + take/drop 管道，可保存/置顶直选；去重建议）

## 快速开始

构建用 [Task](https://taskfile.dev)（单二进制，无需 make）：`brew install go-task/tap/go-task` 或 `go install github.com/go-task/task/v3/cmd/task@latest`。

### CLI（无 GUI 依赖，任何环境可用）

```sh
task build-cli
./build/bin/ntqq-cleaner-cli scan --root <QQ数据根>           # dry-run 统计（默认）
./build/bin/ntqq-cleaner-cli scan --root R --json > m.json    # 生成清单
./build/bin/ntqq-cleaner-cli clean --file m.json --force --backup-dir ~/qq-backup   # 执行（需输入 yes）
```

### GUI

```sh
task build        # 当前主机原生 GUI 单二进制（嵌入工作区现成的 frontend/dist）
task bundle       # macOS .app 打包（需 wails CLI）
task build-windows   # 任意主机交叉出 windows/amd64 exe（纯 Go 后端）
task dev          # 热重载开发（需 wails CLI + pnpm）
```

前端构建（pnpm）：`frontend/dist` 通常已由开发容器构建好并落在工作区（挂载共享），`task build` 检测到 dist 存在会直接嵌入，**宿主机无需 Node/pnpm**。只有改动前端源码后才需要重建（`task frontend --force`，需 pnpm）。

- macOS：系统 WKWebView，无额外依赖；**macOS 目标只能在 Mac 上构建**（ObjC/cgo 需 Xcode 工具链）
- Windows：需 WebView2 运行时（Win10/11 自带）
- Linux：需 `libwebkit2gtk-4.0-dev libgtk-3-dev pkg-config`（编译+运行）

## 安全红线（不可绕过，docs/06）

| 红线 | 实现位置 |
|---|---|
| 默认 dry-run，零写入 | `app.Engine` 只读；`clean.Run` 需 Force+Confirmed |
| 白名单/黑名单逐文件二次校验 | `rules.Whitelisted/Blacklisted` + `clean.VerifyPath` |
| QQ 运行中拒绝清理 | `clean.QQRunning`（开始前 + 每 30s 复查） |
| 删除必留痕：备份移动 或 审计报告 | `clean.deleteOne` + 系统 tmp 下带时间戳的审计文件（清理后自动打开） |
| 前端不可信，红线全在 Go 侧 | `internal/app` 不暴露任意路径/文件能力 |

## 架构

```
discover → classify → reason/关联索引 → report ──(dry-run)──▶ UI 展示
                                              └─(用户确认)──▶ clean
```

| 包 | 职责 | 依据 |
|---|---|---|
| `internal/discovery` | 数据根发现、三源账号识别（mmkv/UnitedConfig/Login，全明文） | docs/01, 02 |
| `internal/classify` | 白名单遍历、文件分类（biz/category/month/md5/size-tag） | docs/01 §2 |
| `internal/rules` | 白名单/黑名单政策 + reason 标签 + 配置 | docs/03 |
| `internal/clean` | 删除执行：进程保护→白名单→备份/SHA-256→审计日志 | docs/06 |
| `internal/app` | Engine（CLI/GUI 共享管线）+ Backend（GUI 绑定，分页/统计/预览） | docs/04 |
| `frontend/` | React 19 + TanStack Virtual：虚拟化照片墙、懒加载缩略图、预览面板 | docs/07 |

## 配置

`config.example.yaml` 含全部默认值（对齐 QQ 官方 3 天规则 259199s）。工具自身的全部文件都在系统临时目录（`<tmp>/ntqq-cleaner/`，本次开机内跨启动复用，OS 自动清理）；GUI 设置对话框可改。

## 测试

```sh
task test                  # 单元 + 集成 + 红线测试（fixture 见 docs/05 §6）
task smoke                 # CLI 端到端冒烟：scan → manifest → clean（红线验证）
```

红线测试覆盖：dry-run 零写入（fixture mtime 全量比对）、黑名单永不入扫描结果、`nt_db` 不清理、无 force/确认拒绝、QQ 运行中拒绝。

## 开发文档索引

| 文档 | 内容 |
|---|---|
| `docs/01_data_structure.md` | 缓存目录结构全解 |
| `docs/02_account_identification.md` | 账号明文识别方案 |
| `docs/03_clean_rules.md` | 清理规则与可清性政策 |
| `docs/04_tool_design.md` | 架构与实现指导 |
| `docs/05_sample_data.md` | 开发机样例（测试验证参考） |
| `docs/06_safety_redlines.md` | 安全红线（硬约束） |
| `docs/07_ui_design.md` | GUI 框架与性能预算 |
