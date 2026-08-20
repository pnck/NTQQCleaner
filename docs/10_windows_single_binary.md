# 10 Windows 单二进制双入口（CUI + 入口分派）

> 请把这个文档交给 LLM agent 阅读

设计目标：Windows 上**一个 exe 同时充当 CLI 与 GUI**——双击 = GUI（零
黑框）；从 PowerShell/cmd 启动 = CLI（裸运行显示 usage，`gui` 子命令
显式进 GUI 且阻塞终端、ops 日志滚动）；`scan`/`clean` 子命令完整可用。

## 1. 机制事实（源码级验证，2026-08）

| 事实 | 依据 |
|---|---|
| shell 等待与否由 PE 头 subsystem 决定：CUI = 等待 + stdio 直连；GUI = 异步 launch-and-forget | [console-allocation-policy 文档](https://learn.microsoft.com/zh-cn/windows/console/console-allocation-policy) |
| CUI exe + manifest 声明 `consoleAllocationPolicy=detached`：24H2+(build 26100) 双击**不分配控制台**（零黑框），shell 仍按 CUI 等待 | 同上 |
| wails CLI 有 `-windowsconsole` flag（"Keep the console when building for Windows"）→ bundle 路径可产出 CUI 子系统 exe | wails v2.14.0 `cmd/wails/flags/build.go:41`、`pkg/commands/build/base.go:257`（`!WindowsConsole` 时才加 `-H windowsgui`） |
| **wails 的 manifest 嵌入会丢弃未知元素**：`winres.AppManifestFromXML` 注释明言「If the xml contains other data, they are ignored」——库解析后从结构体重序列化，`consoleAllocationPolicy` 无法经嵌入阶段进入 exe | winres v0.3.1 `manifest.go:291` |
| winres 库支持**后封**：`LoadFromEXE` + `Set(RT_MANIFEST, rawXML)` + `WriteToEXE`——raw 字节直塞完整 XML，无解析无丢弃；纯 Go，可在 linux 交叉构建机上运行 | winres v0.3.1 `winres.go` |
| winres 库 = wails 的资源嵌入机制本身（go.mod 依赖 v0.3.1，packager.go 使用）——后封工具与 wails 同库同源 | wails v2.14.0 `go.mod:38`、`pkg/commands/build/packager.go:254` |
| 项目级 `build/windows/wails.exe.manifest`（Go 模板：`{{.Name}}`/`{{.Info.ProductVersion}}`）是 wails 的定制点；模板数据解析规则：wails.json 的 info 段，缺省时 CompanyName/ProductName=Name、ProductVersion=1.0.0、Copyright="Copyright........."、Comments="Built using Wails..." | `buildassets.ReadFileWithProjectData`（先读项目目录）、`internal/project/project.go:setDefaults` |
| 启动方式运行时检测：`GetConsoleProcessList` 计数——≥2=父 shell 继承；==1=双击分配的孤儿控制台；0/失败=无控制台（24H2 detached） | 经典手法 |
| 混用自生成 .syso 与 wails build 会撞「too many .rsrc sections」 | wails [issue #4349](https://github.com/wailsapp/wails/issues/4349) |

## 2. 设计

### 2.1 构建链（两条路径都产出 CUI exe）

- **裸 build**（`build:windows-*`）：go build 不再 `-H windowsgui`（默认
  CUI）。
- **bundle**（`bundle:windows-*`）：wails build 加 `-windowsconsole`
  （产出 CUI）。
- **manifest 单一来源**：`build/windows/wails.exe.manifest` 增加
  `consoleAllocationPolicy=detached`（wails 嵌入时会丢弃该元素——
  无妨，见 2.2 后封）。
- 两条路径构建完成后都运行 **`tools/winrespatch <exe>`**（后封，纯 Go）：
  用 winres 库打开 exe → 以 **raw XML** 整体替换 RT_MANIFEST（渲染后
  的同一 manifest，含 detached）→ 补/覆盖 icon（`build/windows/
  icon.ico`）与版本信息（渲染后的 `build/windows/info.json`）——
  裸 build 路径由此补齐资源，bundle 路径幂等覆盖。**工具与 wails
  同库同源，模板文件单一来源**。签名顺序：patch 先于 signtool（当前
  未签名，无冲突）。

### 2.2 入口分派（main）

```
main → setupCrashGuard（docs/09 §3.3）
     → consoleKind()（build tag：dispatch_windows.go / dispatch_other.go）
        ├─ 0 无控制台（24H2 detached 双击）→ GUI
        ├─ 1 孤儿控制台（旧系统双击）→ FreeConsole → GUI
        └─ ≥2 父 shell → CLI 语义：裸运行 = usage（exit 2）；
           scan/clean/gui 子命令照旧（gui = 显式 GUI，CUI 下终端阻塞、
           oplog 滚动——调试形态）
POSIX 行为不变（consoleKind 恒 ≥2，裸运行 = gui 默认）。
```

### 2.3 与 oplog / nativecrash 的关系

- oplog（stdout ops 日志，docs/09 §3.2）在 CUI 下天然完整：控制台
  语境直接可见；双击语境 stdout 无控制台、静默失败零成本。
- nativecrash（SEH/minidump，docs/09 §3.3）不变。

## 3. 测试与验证矩阵

| 层 | 手段 |
|---|---|
| winrespatch | 容器：对 windows 交叉产物跑 patch，`go-winres extract` 验证 manifest 资源含 `consoleAllocationPolicy` 与 comctl32 v6、版本信息正确 |
| 分派逻辑 | windows 交叉编译 + vet；逻辑经 consoleKind 单测（抽象注入） |
| 构建链 | 容器：linux 全量测试 + windows/darwin 交叉 build；CI 矩阵照旧 |
| 真机（用户执行） | ① 双击（24H2）零黑框直接 GUI；② PowerShell 裸运行 = usage；③ `ntqq-cleaner.exe gui` 阻塞终端 + 日志滚动 + `$LASTEXITCODE`；④ `scan`/`clean` 输出正常；⑤ 旧系统双击短暂闪框（FreeConsole，预期） |

## 4. 明确不做

- mt.exe / Resource Hacker 后封（外置 Windows-only 工具，破坏 CI
  交叉构建）；rc.exe/windres 完整资源编译器（重武器）；`-H windowsgui`
  + AttachConsole 混搭（shell 不等待，语义混乱）；watchdog 双进程。

## 5. 参考

- [console-allocation-policy（MS）](https://learn.microsoft.com/zh-cn/windows/console/console-allocation-policy)
- [winres 库](https://github.com/tc-hib/winres) / [go-winres CLI](https://github.com/tc-hib/go-winres)
- [wails #4349 rsrc 冲突](https://github.com/wailsapp/wails/issues/4349)、[#1155 manifest 单一来源讨论](https://github.com/wailsapp/wails/issues/1155)
