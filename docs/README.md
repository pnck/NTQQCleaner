# QQ 缓存清理工具 — 开发迁移包

> 本包是 QQ 缓存清理工具的**开发设计文档包**，内容基于对 QQ 客户端存储/清理逻辑的逆向结论整理。
> 目标：在**纯开发工作区**依据本包实现工具，无需任何逆向能力。

---

## 1. 项目定位

**目标**：为 QQ 桌面版（macOS/Windows）设计一个**跨平台、比 QQ 自带清理更优、更安全**的缓存清理工具。

**形态**：**Go + Wails v2**（内嵌 Web UI，原生窗口）的桌面工具，另提供 CLI 子命令供脚本/CI。默认 **dry-run**（只统计不删除），删除需显式确认（技术栈论证见 `04_tool_design.md` §1）。

**为什么值得做**：
- QQ 缓存可达 **数十 GB**（典型场景：图片/视频/表情缓存长期累积）
- QQ 自带清理**一刀切 3 天**（mtime > 3 天的全算可清理），无价值区分

**核心痛点（调查结论，见 05 §2.1/§4.1）**：
- 缓存大头是**缩略图**而非原图：Pic 的 Thumb 29.1G（占 66%，是 Ori 的 2 倍），Emoji 的 emoji-recv 18.2G 同构
- **缩略图可从原图重建** → 删除缩略图**零数据损失**

**启发式指引**：
- 工具定位主线 = **缩略图删减（零损失清理）**、**按业务类型清理**、**命名筛选器 + 去重建议**——三条功能线**平级**，缩略图删减不是 biz 清理的子项
- 缩略图删减的**现象依据**：可重建性（Ori 在）、多尺寸冗余（`_0`/`_720`）、跨 biz 普遍存在（Pic/Video/Emoji/File 均有 Thumb）
- 具体实现设计由开发者定

---

## 2. 文档索引

| 文档 | 内容 | 用途 |
|---|---|---|
| `01_data_structure.md` | QQ 缓存目录结构全解：路径规则、目录分类、可清理性评级 | 工具遍历与分类的基础 |
| `02_account_identification.md` | 账号识别：`nt_qq_<hash>` 目录 ↔ QQ 号 的明文映射方案 | 工具标注"此目录=QQ号 xxx" |
| `03_clean_rules.md` | 清理规则：QQ 官方 3 天规则、*Temp 规则、类别门控政策 | 清理判定逻辑 |
| `04_tool_design.md` | 工具架构：CLI、模块划分、实现要点、伪代码 | 编码直接参考 |
| `05_sample_data.md` | 开发机样例数据：目录清单、大小分布、文件命名、扩展名统计（非通用，仅验证参考） | 测试与验证用例 |
| `06_safety_redlines.md` | 安全红线：工具必须内置的强制行为（来自原项目 CONSTRAINTS） | 不可省略的硬约束 |
| `07_ui_design.md` | GUI 界面框架：功能区划分、交互逻辑、**媒体快速预览 + 大数据流畅性**（硬性要求） | 界面实现参考 |
| `08_windows_legacy_compat.md` | Windows NTQQ 布局（`<QQ号>/nt_qq/`）、旧版 QQ 兼容层、旧库残留报告的设计与安全红线 | Windows/旧版兼容层实现的依据 |
| `09_windows_move_crash_report.md` | Windows 移动/删除语义重做（占用重试/重启删除登记）、崩溃面包屑、原生崩溃 minidump 兜底 | Windows 真机问题修复的依据 |

> **总纲（交给 LLM agent 阅读）**：上手路径、代码地图、开发流程约定见
> [`CONTRIBUTING.md`](CONTRIBUTING.md)（本包所有文档的导航入口）。

---

## 3. 关键事实速查（开发必须知道的硬结论）

### 3.1 数据目录结构

`<QQ>` = QQ 数据根目录（**自动探测或 `--root` 指定**，跨平台默认位置见下表）：

| 平台 | 默认位置 |
|---|---|
| macOS | `~/Library/Containers/com.tencent.qq/Data/Library/Application Support/QQ` |
| Windows | `Documents\Tencent Files`（真机实测主根；`%APPDATA%\Tencent\QQ` 为候选）——布局与探测见 `08_windows_legacy_compat.md` |

```
<QQ>/
├── global/                      # 全局（跨账号）数据
│   ├── nt_data/                 # 全局缓存/配置（Login 标记、mmkv、UnitedConfig）
│   ├── nt_db/                   # 全局加密库（login.db 等，读取需 dump enc_key，工具不清理）
│   └── nt_temp/
├── nt_qq_<32hex>/               # 每个账号一个实例目录
│   ├── nt_data/                 # ★ 缓存主体（清理工具主要目标）
│   ├── nt_db/                   # 账号加密库，工具不清理
│   └── nt_temp/                 # 临时文件，可安全清理
└── <数字QQ号>/                   # 旧版明文目录（登录前旧格式，含 msg3.0.db）
```

> **Windows NTQQ 布局不同**：实例目录是 `<QQ号>/`（明文），nt_* 组在
> `<QQ号>/nt_qq/` 下，全局目录是 `<根>/nt_qq/global/`；实例目录顶层还
> 残留旧版库（Msg3.0.db 等，只统计不清理）。详见 `08_windows_legacy_compat.md`。

### 3.2 账号识别（完全明文，无需解密）
```
nt_qq_<32hex>  ↔  QQ <号码>     # 示例：任意账号目录均可识别
```
来源：`global/nt_data/mmkv/mmkv.default` 明文路径 key、`nt_qq_*/nt_data/UnitedConfig/` 子目录名、
`global/nt_data/Login/.<qq号>` 标记文件。详见 `02_account_identification.md`。

### 3.3 nt_data 缓存目录（可清理性）

> 参考大小列来自开发机一次实测，仅示意数量级，**非通用数据**。

| 目录 | 内容 | 参考大小* | 可清理性 |
|---|---|---|---|
| `Pic/` | 图片缓存，`{YYYY-MM}/{Ori|Thumb}/{md5}[_{size}].{ext}` | ~44G | ★★★ 主要目标 |
| `Emoji/` | 表情（5 子类） | ~18G | ★★★（分细类） |
| `Video/` | 视频缓存，同 Pic 结构 | ~1G | ★★★ |
| `File/` | 文件传输，**无年月**，`{Ori|Thumb|ThumbTemp|file_assistant}` | ~200M | ★★ |
| `Ptt/` | 语音，`{YYYY-MM}/{Ori|OriTemp}` | ~7M | ★★ |
| `dataline/` | 数据线 | ~50M | ★★ |
| `log/` `log-cache/` | 日志 | ~400M | ★★（另类处理） |
| `avatar/` | 头像缓存 | ~100M | ★ |
| `mmkv/ msf/ OnlineStatus/ ...` | 配置/状态 | 小 | ❌ 工具不清理 |

### 3.4 QQ 官方清理规则（逆向结论，直接作为默认值）
- **3 天规则**：`now - file_mtime > 259199 秒` 的文件才算可清理（`259199 ≈ 3 天`）
- **mode_flag==0** 时全部文件可清（"清空"模式）
- **\*Temp 目录**（`OriTemp/ThumbTemp`）= 下载中断残留，QQ 单独优先清理
- **引用判定**：QQ 通过解析**加密 DB** 提取消息里的文件引用（md5/uuid）→ 工具**不解析 DB**
  （解析需运行时 dump enc_key）→ 只能靠文件系统启发式近似（详见 `03_clean_rules.md`）

---

## 4. 新工作区建议结构

```
ntqq-cleaner/
├── main.go                 # 入口：wails.Run + Bind + embed.FS
├── internal/
│   ├── discovery/          # 数据目录发现 + 账号识别
│   ├── classify/           # 遍历 nt_data + 文件分类
│   ├── rules/              # 白名单判定 + reason 标签
│   ├── report/             # 统计与报告模型
│   ├── clean/              # 执行删除（红线：备份/确认/进程保护）
│   └── app/                # 前端 Backend（Bind 到 window.go.backend）
├── frontend/               # Wails 前端（Vite，embed.FS）
├── tests/
├── go.mod
└── Makefile
```

完整实现细节见 `04_tool_design.md`。

---

## 5. 迁移交接说明

- **迁移方式**：将整个 `devkit/` 目录**原样复制**到新开发工作区即可（无需打包）
- **逆向工具链**（IDA/wrapper.node/SQLCipher 编译产物）**不再需要**，新工作区勿引入
- **加密 DB 工具一律不清理**（`nt_db/`、`*.db`、`*.db-wal`、`*.db-shm`）——读取需运行时 dump enc_key（离线不可直接解析）
- 文档中的结构/规则/算法结论可直接作为实现依据；`05_sample_data.md` 为开发机样例数据（仅作验证参考）
- 开发时**遵守 `06_safety_redlines.md`**，默认 dry-run 是铁律；技术栈见 `04_tool_design.md` §1；界面见 `07_ui_design.md`
