# 06 — 安全红线（工具必须内置的强制行为）

> 源自逆向项目的 CONSTRAINTS.md，但**只保留工具自身必须实现的红线**。
> 这些是工具的**硬性要求**，任何模式都不允许绕过，代码评审第一优先级。
> **实现原则（Go + Wails）**：UI 层视为**不可信输入**，所有红线在 Go 侧强制实现；
> 前端只负责展示与触发，不能作为安全边界。

---

## 1. 默认 dry-run（铁律）

- 工具**默认只统计、列出**将被清理的路径与大小，**不删除任何文件**
- 只有显式传 `--force` 才允许真正执行，且执行仍需二次确认
- dry-run 模式下**零写入**（连日志文件都只在明确开启时写）

## 2. 白名单/黑名单路径

- 只操作 `nt_data/` 下明确列入"可清理"白名单的目录：
  - 富媒体缓存：`Pic/ Video/ Ptt/ File/ dataline/ Emoji/emoji-recv/ Emoji/BaseEmojiSyastems/ThumbTemp/`
  - 超龄归档月目录（按配置）
  - `*Temp` 目录（`OriTemp/ ThumbTemp/`）与 `dataline/.tmp/` 传输残留
  - `log/ log-cache/` 运行日志（clean_log 门控，QQ 自动重建）
- **工具永不删除/修改**（黑名单，代码里硬编码 + 运行时二次校验；「不删除」≠「不可读」——加密库读取属 dump enc_key 能力，不在本工具范围）：
  - `mmkv/ msf/ OnlineStatus/ UnitedConfig/ config/ avatar/`
  - 任何 `*.db` `*.db-wal` `*.db-shm` `*.db-first.material` `*.db-last.material`
  - `nt_db/`（账号与全局）
  - 账号目录根以外的任何文件
- 每个待删文件在删除前**再次校验**：其路径必须匹配白名单正则，否则跳过并告警

## 3. 删除前强制备份

- 优先 `--backup-dir`：被删文件**移动到**备份目录（移动 = 天然可恢复，比删除安全）
- 无备份目录时：必须记录**完整清单 + SHA-256 校验和 + 原路径**到日志文件
- 未记录就删除 = 程序 bug，禁止

## 4. `--force` 显式参数 + 二次确认

- 没有 `--force` 参数 → 工具拒绝任何删除
- 有 `--force` → 仍须交互输入 `yes` 确认（打印待删数量与总大小）

## 5. 进程保护

- 执行前检查 QQ 是否运行：`pgrep -f "QQ.app"`（macOS）/ `tasklist` 过滤（Windows）
- 若 QQ 运行中 → **拒绝清理并退出**（QQ 可能正在读写缓存文件）
- 该检查在删除前**最近时刻**再做一次（防止检查后 QQ 启动）

## 5b. Web UI 层安全（Go + Wails 特有）

- **UI 不可信**：前端传来的路径/参数一律校验，不信任前端之外的内容
- **无远程内容**：Wails 只加载 `embed.FS` 本地资源，禁止加载 http(s) 外部页面；Wails 原生 IPC 不经网络 → 天然无 CSRF
- **Bind 白名单**：只暴露 `Scan/Clean/GetConfig/SetConfig/DiscoverRoots`，不暴露文件系统泛化能力
- **长任务不阻塞 UI**：扫描在独立 goroutine，通过 `runtime.EventsEmit` 推进度；Clean 请求带显式确认标志
- **CLI 与 GUI 同安全**：`clean` 子命令与 GUI 走同一 clean 包，红线逻辑一致

## 6. 只读 QQ 数据（工具行为）

- 工具**只读** QQ 数据目录（含 mmkv/Login/UnitedConfig 等账号识别来源）——「只读」指**不写不删**，不指无法访问
- 不修改、不写入任何 QQ 目录内文件
- 加密库（`nt_db/*.db`）读取需运行时 dump enc_key（社区工具如 `qq_key_for_mac`），**不在本工具范围**
- 不触碰 `/Applications/QQ.app`、系统目录
- 不执行 QQ 二进制

## 7. 运行环境限制

- 无需管理员权限、无需 `sudo`、无需系统级安装
- 仅标准库，无第三方依赖（避免破坏系统 Python 环境）
- 若提示"需要安装依赖"→ 应使用工作区 venv，**不**系统级安装

---

## 8. 代码评审检查清单

- [ ] 删除逻辑只有 `clean.py` 一处，且必须经过：进程保护 → 白名单校验 → 备份 → 二次确认
- [ ] `--force` 未传时，任何 `unlink/move/rm` 都不可达
- [ ] 黑名单目录名在遍历层与删除层**双重**过滤
- [ ] dry-run 对真实目录零写入（可测试验证 mtime 不变）
- [ ] 报告里明确标注每个文件的 reason 标签（展示说明）
- [ ] 无网络请求、无外联（工具完全离线）
