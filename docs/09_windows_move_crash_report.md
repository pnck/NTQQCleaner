# 09 Windows 移动/删除语义重做与崩溃报告

> 请把这个文档交给 LLM agent 阅读

本文档是 Windows 真机实测暴露的两个问题的根因分析与修复设计：
① 勾选「移动代替删除」时批量清理遭遇文件占用错误；② 连续批量清理
触发进程静默消失（无报错、无事件查看器记录、无系统 bugreport）。

权威性：本文档的修复设计变更 platform/clean 层语义与 UI 报告契约，
红线不变（docs/06 全文仍为第一优先级）。

## 1. 实测事实（2026-08 真机）

| # | 观察 | 环境事实 |
|---|---|---|
| 1 | 勾选「移动代替删除」批量清理，原文件报「文件占用」错误 | 备份目录与数据目录**跨分区**；**QQ 正在运行**（经二次确认 IgnoreRunning 清理） |
| 2 | 连续批量清理极容易触发 app 消失 | `%TEMP%\ntqq-cleaner\crash-*.log` **全部 0 字节**；事件查看器无任何条目；release 无 bugreport 弹窗 |

## 2. 根因分析

### 2.1 文件占用错误（跨分区 + QQ 持锁）

当前链路：`platform.MoveFile` → `RenameOrCopy`（files.go）：

1. `os.Rename(src, dst)`——Windows 上即 `MoveFileExW(src, dst,
   MOVEFILE_REPLACE_EXISTING)`，**无 `MOVEFILE_COPY_ALLOWED`**；
2. 备份目录跨分区 → `MoveFileExW` 恒失败 `ERROR_NOT_SAME_DEVICE` →
   每个文件都走「打开源→完整复制→`os.Remove(源)`」兜底；
3. QQ 正在运行，以**不含 FILE_SHARE_DELETE 的共享模式**持有缓存文件
   句柄 → 复制成功（读共享允许），`os.Remove(源)` 失败
   `ERROR_SHARING_VIOLATION`（即中文系统报错「文件被占用」）。

两个已排除项（实测/语义确认）：

- **本进程不自锁**：Go 的 `os.Open` 在 Windows 一律以
  `FILE_SHARE_READ|FILE_SHARE_WRITE|FILE_SHARE_DELETE` 打开，预览
  服务器/首帧解码/复制读源的句柄不会挡住自己 rename/delete。
- **只读属性已处理**：现有 `os.Chmod(0o666)` 重试覆盖该情形，与
  占用错误无关。

占用者结论：外部进程（QQ 本体/资源管理器缩略图/杀软/Search 索引器）。
「移动」模式比「删除」模式更易暴露，因为跨卷时全部走复制→删源，
大文件复制有秒级窗口，且出现「复制成功、删源失败」这种删除模式
没有的中间态。

### 2.2 进程静默消失（0 字节 crash log）

`internal/logring` 的 crash 文件只在崩溃瞬间才写入（环形缓冲转储/
运行时崩溃转储）。**全部 0 字节 = 进程死亡完全没有经过 Go runtime
的崩溃路径**（非 panic、非 fatal）。两个可能：

- **外部 TerminateProcess**：QQ 安全防护组件/杀软对「批量删除 QQ
  数据目录」的进程的干预——时机与用户操作完全吻合，为首要嫌疑；
- **非 Go 线程上的原生崩溃**：Go runtime 的 vectored handler 管不到
  的线程（如 WebView2 宿主进程内线程）发生 AV。

「事件查看器无条目、无 bugreport 弹窗」本身是**设计行为而非缺陷**：
`debug.SetCrashOutput` 接管 Go panic 后直接 `ExitProcess(2)`，WER
不介入。当初的决策「原生崩溃由 WER 默认开启覆盖，无需代码」
（logring 包注释）在该真机上被证伪。

## 3. 设计

### 3.1 Windows 适配层：删除/移动语义重做（platform 层）

删除与移动统一为**Windows 原生语义 + 三级阶梯**（全部纯 syscall，
x/sys/windows 已有 MoveFileEx/DeleteFile，容器可交叉编译）：

```
删除：DeleteFileW
  → ERROR_SHARING_VIOLATION(32)/ERROR_LOCK_VIOLATION(33)：
    退避重试 3 次（100ms/500ms/2s，杀软/缩略图锁多为瞬态）
  → 仍失败：MoveFileExW(src, nil, MOVEFILE_DELAY_UNTIL_REBOOT)
    登记重启删除（需管理员；非管理员返回原错误）
  → 成功登记：返回 platform.ErrDeferredReboot（哨兵，errors.Is）

移动：MoveFileExW(src, dst, MOVEFILE_REPLACE_EXISTING)
  → ERROR_NOT_SAME_DEVICE(17)：跨卷 → 复制 + 删源
    （删源同「删除」三级阶梯；备份副本已生成后再登记删源不算
    降级——用户要的备份已在，原文件只是延后消失）
  → 32/33：退避重试；仍失败且同卷：
    MoveFileExW(src, dst, REPLACE_EXISTING|DELAY_UNTIL_REBOOT)
    登记重启移动 → ErrDeferredReboot
```

契约（platform 包导出）：

- `platform.ErrDeferredReboot`：`errors.Is` 判断；clean 层识别后
  `CleanItem.Action = "reboot"`，审计记录 `action: "reboot"`，
  `Result.RebootDeferred` 计数（不进 Failed）。
- 只读属性先行清除（保留现逻辑）。

理由：持久锁（QQ 持锁）重试无法解决；重启删除是 Windows 处理被占用
文件的官方机制（PendingFileRenameOperations）。QQ 运行中清理
（IgnoreRunning）从「尽力删除+报占用」升级为「即时可删的删、
被占用的登记重启删」，UI 如实报告数量。

### 3.2 崩溃面包屑（logring.Crumb）

crash 文件从**启动起持续落 breadcrumb**（区别于仅在崩溃瞬间写入的
现状）：启动（pid/版本）、scan 开始/结束、clean 开始/每 1000 文件
进度/结束。外部击毙时，最后一根面包屑直接定位死点与进度；0 字节
文件从此不再出现（启动即落首行）。

### 3.3 原生崩溃兜底（Windows-only）

`internal/logring/native_windows.go`（build tag windows）：

- `EnableCrashLog` 时预加载 dbghelp 的 `MiniDumpWriteDump` proc 并
  `kernel32.SetUnhandledExceptionFilter` 安装过滤器；
- 触发时：写 minidump（`MiniDumpNormal|MiniDumpWithIndirectly
  ReferencedMemory`，**不用 FullMemory**——进程内存可能含 QQ 账号/
  路径之外的敏感数据）+ `DumpCrash()` 把环形缓冲追加进 crash log；
- 返回 `EXCEPTION_CONTINUE_SEARCH`：链回默认处理器，**系统 WER/
  bugreport 按用户系统策略正常介入**——补上「WER 默认开启」假设
  失败的那一环；
- 与 Go runtime VEH 共存：Go panic/fatal 走 SetCrashOutput（现状
  不变），本过滤器只接住 VEH 不处理的非 Go 线程异常；两条链路
  不冲突。

### 3.4 UI 报告契约扩展

- `CleanItem.Action` 联合 += `"reboot"`；Backend items 过滤条件
  += reboot（与 skip/fail 一同回传）；
- `CleanResult` += `rebootDeferred` 计数；清理结果对话框摘要行与
  badge 增加「重启后删除」；
- 审计 `action` 字段取值 += `reboot`。

## 4. 安全红线对照（docs/06）

- **不变**：reboot 登记发生在 clean.Run 的逐文件白名单/黑名单重验
  **之后**；dry-run 零写入（扫描不触碰本节任何新路径）；无
  --force + 确认不可删；
- **新增的边界**：reboot 登记写系统 PendingFileRenameOperations，
  生效在下次启动——文件在重启前仍在原地，属于「删除的延期」而非
  新能力；审计照常记录；
- minidump 不含完整内存（§3.3），与审计/崩溃日志同存于
  `%TEMP%\ntqq-cleaner\`，隐私级别不高于既有 crash log。

## 5. 测试与验证矩阵

| 层 | 手段 |
|---|---|
| clean 层 reboot 分支 | 假 Adapter（platform.Install）返回 ErrDeferredReboot → 断言 Action/计数/审计 |
| logring.Crumb | 单测：启动即落首行、进度行追加进同一文件 |
| Windows 适配层编译 | `GOOS=windows CGO_ENABLED=0 go build ./...`（容器门禁） |
| 真机（用户执行） | ① 完全关闭 QQ 复测批量清理——若闪退消失则坐实外部击毙；② 跨卷移动+QQ 运行：占用文件进「重启后删除」计数；③ 重启后确认 PendingFileRenameOperations 生效；④ 杀软白名单/Defender 排除后复测 |

## 6. 待确认项

- `MOVEFILE_DELAY_UNTIL_REBOOT` 需管理员权限——非管理员时行为退化为
  现状（报占用错误）；如真机非管理员环境占比高，后续可评估用户态
  持久锁释放（RstMgr）方案，本期不做。
