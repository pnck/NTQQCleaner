# 08 — Windows 与旧版兼容层设计

> 本文档定义 ntqq-cleaner 的 **Windows 兼容层**与**旧版 QQ 兼容层**：
> 现状（2026-08-15）是知识层只有 NT 实现的 mac 布局，Windows 布局与
> 旧版布局 fail-closed 到 generic（不可识别、不可扫描）。本文档把两者
> 升级为「可识别 + 可报告」，扫描/清理权限维持 fail-closed 红线不变。
>
> 逆向事实来源：逆向 agent 的 mac QQ 6.9.99 ↔ Windows NTQQ 9.9.32
> 双向对比 + 真机（WSL）数据目录实测（2026-08-15 情报交换），以及
> docs/01-02 的 mac 逆向结论。标注「待确认」的条目等待逆向 agent 补充。

---

## 1. 目标与范围

| 层 | 现状 | 目标 |
|---|---|---|
| Windows NTQQ 布局 | 未实现：probe 只认 `nt_qq_<32hex>`，根候选只有 `%APPDATA%\Tencent\QQ` | 识别 `<QQ号>/nt_qq/…` 实例布局与 `nt_qq/global` 全局布局，探测 `Documents\Tencent Files` 根，扫描/分类/清理全链路可用 |
| 旧版 QQ（数字目录 + msg3.0.db） | 无实现（注释占位），fail-closed 到 generic | 新增 `impl/legacy`：识别布局、识别账号、**只统计不扫描不清理**（fail-closed 语义升级为「可报告」） |
| Windows 账号内的旧库残留（Msg3.0.db 等 38 库） | 不感知 | 报告残留占用（与 QQ 官方清理器一致：只统计不清理） |

非目标：解析/清理加密 DB（红线不变）；旧版缓存目录的扫描（无逆向结论，
保守不扫）；Windows 以外的旧版清理。

---

## 2. 逆向结论：三套布局对照

### 2.1 数据根

| 平台 | 默认根候选（探测顺序） |
|---|---|
| macOS | `~/Library/Containers/com.tencent.qq/Data/Library/Application Support/QQ`（docs/01；逆向 IDB + 历史真机 64+ 处实证复核确认，`Data/Documents` 是误写已否决）。IDA 佐证：GetSaveDir 根来自运行时注入的 biz_base_dir，非硬编码——探测与发现逻辑兼容 |
| Windows | ① `<Documents>\Tencent Files`（真机实测主根；Documents 可能被重定向）；② `%APPDATA%\Tencent\QQ`（**次级回退**：binary + 全新安装真机双证据确认非数据根——9.9.33 wrapper.node 无 RoamingAppData 路径拼装（SHGetFolderPathA 只用 ProgramData；Tencent\QQ 命中仅注册表与旧版兼容路径模板），全新安装只产生空 STemp\Uninstall 残留。仍保留回退以覆盖极特殊情况（升级路径/旧版兼容代码），探测要求目录存在且含实例才选中，零风险） |
| Linux | 无稳定默认根，`--root` / UI 指定（现状不变） |

### 2.2 账号实例与全局目录（Windows 与 mac 差异最大处）

| | macOS NT（docs/01） | Windows NTQQ（实测） |
|---|---|---|
| 实例目录 | `<根>/nt_qq_<32hex>/` | `<根>/<QQ号>/`（QQ 号明文） |
| nt 子目录组 | 直接在实例目录下：`nt_data/ nt_db/ nt_temp/` | 在 `nt_qq/` 子目录下：`<QQ号>/nt_qq/{nt_data,nt_db,nt_temp}` |
| 实例目录顶层其它内容 | 无 | **旧版库残留**：Msg3.0.db（实测 10.6GB）等 38 个旧库，迁移后停更；QQ 官方清理器只统计不清理 |
| 全局目录 | `<根>/global/` | `<根>/nt_qq/global/` |
| Login 标记 | `<根>/global/nt_data/Login/.<QQ号>` | `<根>/nt_qq/global/nt_data/Login/.<QQ号>` |
| mmkv | `<根>/global/nt_data/mmkv/mmkv.default` | 同上（路径带 nt_qq/）。**key 为明文路径形态**（非 mac 的 hash 形态）：`key=flash_transfer_document_dir_key`，`value="A@<盘符>:\documents\Tencent Files\<QQ号>\nt_qq\nt_data\flashfransfer"`（注意 `A@` 前缀）；另有 `<QQ号>_flash_transfer_document_cache_key` 键 |

### 2.3 nt_data 缓存布局

Windows 与 mac **一致**：`nt_data/{Pic|Video|Ptt|File|Emoji}/{YYYY-MM}/{Ori|OriTemp|Thumb|ThumbTemp}/{md5}[_size].ext`，
Emoji 五子类、File 无年月、dataline/.tmp 等结构全部同构（docs/01）。
已知差异：FilterVideo 目录仅 mac 有（Windows elem 49 返回 0）；chat_type
位图差异只影响 DB 解析，与本工具无关。**待确认**：Windows `nt_data` 下
是否有 mac 没有的目录（出现即不入白名单，天然 fail-closed，仅影响报告完整性）。

### 2.4 旧版 QQ（数字目录 + msg3.0.db）

- 形态：`<根>/<数字QQ号>/`，顶层 25 个旧库 + 缓存目录（实测清单）。
- 判定：数字目录且**不含** `nt_qq/nt_data`（含则它是 Windows NT 实例）。
- 官方行为（Windows IDB 实证，2026-08-15）：**官方清理工具不清理任何
  旧版遗留**——CacheScannerWorker 的扫描/清理路径集合只含 nt_data biz
  子目录 + flashfransfer 三子目录，无旧版顶层目录；`ScanQQDirSize` 构造
  旧库（Msg3.0.db）+ 新库（nt_msg.db）双路径但**只累加 size 不删除**
  （「保留 + 统计」策略，迁移后靠用户手动清理）。本工具的「只统计不
  清理」与官方策略完全一致 ✓。旧版遗留作为**独立冗余展示**（CLI 报告 /
  GUI 账号行标记），不并入任何可清理 category，也不提供清理门控
  （比官方更保守：连统计之外的操作都没有）。
- 实测顶层缓存目录：`Ads AppWebCache Audio CloudRes DiscussHead Emoji
  ExpressionRecommend FileRecv GroupCollection Image MyCollection
  OfflinePackage QQ QQGuild RecommendFace SelCreateGroupHead Video
  Wireless VasscUpdate` + 25 个 .db。
- **旧版命名与 NT 完全不同**（docs/01 的 `{md5}[_{size}].{ext}` 不适用）：
  - `Image/` 分类子目录（C2C Group Group2 HistroyHead MarktingMsgCachePic
    MsgWander PhotoWall PicFileThumbnails SharePic Thumbnails），C2C 内再分
    Image1/…；
  - Image/Audio 文件名是 `$` 开头的编码名（如 `$$5SQ@VGFJQ486@L7Y2~PSK.gif`）；
  - `Video/` 为 md5.png 平铺；**无年月目录**。
  - ⇒ 旧版若要做扫描需要一套独立的知识（解析/分类/白名单），v1 不做。
- 旧库实测 25 个（此前记 38 是含 -journal 与全账号合计）：`Msg3.0.db`
  （10.6GB）`Msg3.0index.db`（2.2GB）`FaceStore.db Info.db Misc.db
  MiscHead.db CustomFace.db Registry.db Registry2.0.db manage.db
  Thumbnails.db SmallEmoji.db RE.db FileInfo.db FileShare1.0.db
  FileTransferMsg.db FoldMsg.db GroupActiveStatus.db PubAcc2.0.db
  ApprovalStorage.db DingDongStorage.db DocImport.db ExpressionRecommend.db
  Infocenter.db TencentDocRecord.db`。报告按实际目录内容统计，不写死名单。
- mac 6.9.99 已是 NT 架构，**未发现旧库残留** → 旧版布局仅 Windows/老
  mac QQ；legacy probe 按纯目录形态跨平台实现（mac 上不会命中）。
- 注：根下还可能有 `All Users`、`nt_qq`（global 所在）等非数字目录，
  以及 TIM 同构产品（All Users\QQ 旧 Registry2.0.db 暗示多产品共存）——
  TIM 为文档记录的非目标。

---

## 3. 设计

### 3.1 总原则

1. **fail-closed 红线不变**：任何新增实现拿不到扫描/清理权限的，其
   `ScanCapable()` 一律 false；白名单结构之外的文件永不入索引/永不被删。
2. **兼容层 = 识别 + 报告**：旧版与旧库残留只产生统计输出，绝不产生
   删除动作；「只统计」是 QQ 官方对齐的行为，也是红线「报告不撒谎」的
   要求（用户看到 10GB 旧库却什么都扫不到会以为工具坏了）。
3. **上层零改动**：discovery/classify/rules/clean 继续只依赖
   `qq.Knowledge`；差异全部收敛在 impl 包内。

### 3.2 布局差异抽象：`qq.Instance.NtRel` + nt 包 per-OS spec

`qq.Instance` 增加一个字段：

```go
type Instance struct {
    DirName string
    Hash    string
    // NtRel 从实例目录到 nt_* 子目录组的相对路径：
    //   NT macOS: ""（nt_data 直接在实例目录下）
    //   NT Windows: "nt_qq"（<QQ号>/nt_qq/{nt_data,nt_db,nt_temp}）
    //   旧版: ""（无 nt_* 组）
    NtRel string
}
```

discovery 把硬编码的 `filepath.Join(root, inst.DirName, "nt_data")` 改为
`filepath.Join(root, inst.DirName, inst.NtRel, "nt_data")`（nt_temp/nt_db
同理）。mac 行为不变（NtRel 零值 ""）。

nt 包内部新增 **layoutSpec**（布局差异点集合），逻辑本体（探测/枚举/
识别/分类/白名单）跨平台共享，仅 spec 随 OS 不同：

```go
// layout.go（无 build tag，可被任意 OS 的测试直接引用）
type layoutSpec struct {
    // instance 判断根目录条目是否为 NT 实例目录；返回 hash 键。
    // mac: nt_qq_<32hex> → 32hex；windows: ^\d{5,12}$ 且含 nt_qq/nt_data → 目录名（即 QQ 号）
    instance func(root, name string, isDir bool) (hash string, ok bool)
    ntRel     string // mac: ""；windows: "nt_qq"
    globalRel string // mac: "global"；windows: "nt_qq/global"
    dirIsQQNum bool // windows: true（目录名即 QQ 号，识别直接返回）
}
var currentSpec = darwinSpec // spec_<os>.go 的 init 覆盖为当前 OS 的 spec
```

- `spec_darwin.go` / `spec_windows.go` / `spec_linux.go`（build-tagged）
  只做 `func init() { currentSpec = xxxSpec }`。
- **linux 容器可测 windows 行为**：nt 包测试与 spec 同包，直接
  `currentSpec = windowsSpec`（defer 恢复）即可在 linux 上验证 Windows
  布局的探测/枚举/识别/残留逻辑——这是把差异做成数据而非代码分支的
  直接收益。

### 3.3 探测与实例枚举（probe）

- **nt probe**（跨平台共享）：`InstanceDirs(root)` 命中 ≥1 即返回
  `&NT{}`。枚举逻辑走 `currentSpec.instance`：
  - mac：目录名匹配 `^nt_qq_([0-9a-f]{32})$`（现状不变）。
  - windows：目录名匹配 `^\d{5,12}$` **且** `<root>/<name>/nt_qq/nt_data`
    是目录。第二个条件是关键判别：Windows 根下数字目录可能只是旧版残留
    （无 nt_qq/nt_data），不能被 NT 认领。
- **legacy probe**：根下存在 ≥1 个数字目录（`^\d{5,12}$`）**且不含**
  `nt_qq/nt_data`。与 nt 的 Windows 判定互斥互补。
- **两遍 Detect 已覆盖混存场景**（docs 已有测试 TestDetectPrefersCapable）：
  同根混存 NT 实例与旧版残留时，ScanCapable 的 nt 胜出；纯旧版根 →
  legacy（不可扫描，但可报告）；什么都认不出 → generic。
- `qq.Detect` 与 `qq.InstanceDirs` 的接口签名不变，probe 链注册顺序
  仍与优先级解耦。

### 3.4 账号识别

- **Windows NT**：实例目录名即 QQ 号（明文），直接返回 `DirName`。
  mmkv 为明文路径形态（key 值含 `<QQ号>\nt_qq\nt_data\flashfransfer`，
  带 `A@` 前缀，§2.2）——作为未来交叉验证候选记录，v1 不实现。
- **mac NT**：三源现状不变（mmkv → UnitedConfig），mmkv 路径改用
  `spec.globalRel`。
- **legacy**：目录名即 QQ 号。

### 3.5 旧版残留报告：`qq.ResidueReporter` 可选接口

```go
// qq 包
type LegacyResidue struct {
    Path  string // 绝对路径（目录或文件）
    Size  int64  // 只 stat 汇总，不读文件内容
    Count int    // 文件数
}
type ResidueReporter interface {
    Residues(root string, inst Instance) ([]LegacyResidue, error)
}
```

- **NT Windows** 实现：遍历实例目录顶层条目，**排除 `ntRel`（"nt_qq"）**，
  逐条统计大小/文件数 → 旧库残留清单（Msg3.0.db 等）。只读、不跟进
  符号链接。
- **legacy** 实现：实例目录顶层条目全量统计（整个目录就是旧版数据）。
- **NT mac / generic**：返回 nil（无残留）。
- 消费方：
  - `discovery.Account` 增加 `LegacyResidues []qq.LegacyResidue`；
  - CLI `scan` 报告每账号一行「旧版残留 X 项 · Y GB（只统计不清理）」；
  - 纯旧版根：`scan` 先报告旧版账号与占用，再以明确错误退出
    （「unsupported QQ data layout (detected: legacy)」+ 报告）；
  - GUI：`AccountReport` 增加 `legacyResidueSize`/`legacyResidueCount`，
    账号行展示小标记（前端最小改动）。
- **红线**：Residues 的输出只进报告，绝不进 clean.Request；clean 的
  AllowedRoots 校验不含任何残留路径，即使误传也无法删除（路径不在
  nt_data 白名单根内即拒）。

### 3.6 Windows 根路径探测

`roots_windows.go` 两个候选（§2.1）：`<Documents>\Tencent Files` +
`%APPDATA%\Tencent\QQ` 次级回退（binary 证据确认非数据根，仅为极特殊
情况兜底；存在且含实例才会被自动探测选中）。Documents 定位链（按序，
全部失败才放弃；`--root` 覆盖始终可用）：

1. `SHGetKnownFolderPath(FOLDERID_Documents)`（shell32，经
   `golang.org/x/sys/windows` 的 GUID/CoTaskMemFree + syscall LazyDLL，
   无需 cgo）——最权威的重定向解析（OneDrive/改盘符）；
2. 注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\User
   Shell Folders` 的 `Personal` 值（`%VAR%` 展开）；
3. 环境变量 `Documents`（实测机上该变量即指向重定向盘符）；
4. `%USERPROFILE%\Documents`。

自动探测仍按「第一个 `IsInstanceRoot` 通过的候选」选取（main.go/
backend.go 现状逻辑不变）——加了 Windows 实例判定后，`Tencent Files`
根会被正确识别。

### 3.7 进程守卫（Windows）

实测：Windows NTQQ 主进程 = `QQ.exe`（多个 Electron 子进程同名），另有
`QQEX.exe` 子进程；旧版 QQ 进程名同为 `QQ.exe`（靠安装路径区分）。
⇒ `platform/adapter_windows.go` 现状的 `tasklist /FI "IMAGENAME eq QQ.exe"`
已覆盖 NTQQ 与旧版，无需变更；QQEX.exe 是 QQ.exe 的子进程，不需要单独
匹配。守护语义（清理前最后时刻复查）不变。mac QQ 二进制进程名待逆向
确认（darwin 适配器现状 `QQ.app/Contents/MacOS/QQ`）。

### 3.8 文件清单

| 文件 | 变更 |
|---|---|
| `internal/qq/qq.go` | `Instance.NtRel`；`LegacyResidue` + `ResidueReporter`；包注释更新 |
| `internal/qq/impl/nt/layout.go` | 新增：layoutSpec + darwinSpec/windowsSpec/linuxSpec + currentSpec |
| `internal/qq/impl/nt/spec_{darwin,windows,linux}.go` | 新增：build-tagged init 设 currentSpec |
| `internal/qq/impl/nt/nt.go` | InstanceDirs/Identify 走 spec；新增 Residues（Windows 语义） |
| `internal/qq/impl/nt/probe.go` | probe 走 spec（签名不变） |
| `internal/qq/impl/nt/roots_windows.go` | 重写：Documents\Tencent Files（SHGetKnownFolderPath 定位链）+ APPDATA 次级回退；新增 knownfolder_windows.go |
| `internal/qq/impl/nt/roots_darwin.go` | 不改（逆向复核确认现状路径正确） |
| `internal/qq/impl/legacy/legacy.go` | 新增：Legacy 实现（fail-closed）+ probe + Residues |
| `internal/qqimpl/qqimpl.go` | 注册 legacy probe |
| `internal/discovery/discovery.go` | NtRel 拼接；残留填充 |
| `internal/report/report.go` | AccountReport 增加 legacy 字段 |
| `main.go` | scan 报告旧版残留；纯旧版根的专门提示（printLegacyReport）；--account 用法说明 |
| `internal/platform/adapter_windows.go` | 修复存量编译错误（err 作用域越界——该文件此前从未编译通过，交叉编译暴露） |
| `frontend/src/types.ts` `components/TopBar.tsx` | AccountReport 加 legacy 字段；账号下拉显示「旧版残留 X」 |
| `internal/qq/impl/nt/layout_test.go`（新） | Windows spec 的探测/枚举/识别/残留/Discover 测试（linux 上换 spec 运行） |
| `internal/qq/impl/legacy/legacy_test.go`（新） | legacy 枚举/识别/残留/互斥/fail-closed 测试 |
| `internal/qq/detect_test.go` | TestDetectLegacyOnly 期望从 generic 更新为 legacy（仍 incapable） |
| `go.mod` | golang.org/x/sys 转直接依赖（windows registry） |
| `docs/README.md` | 索引表加 08；§3.1 Windows 布局注记 |

---

## 4. 安全红线（本兼容层新增/强化）

1. 旧版与旧库残留**只统计**：任何模式下都不可进入删除路径；Residues
   输出不参与 clean 校验。
2. Windows 数字目录判定**双向互斥**：含 `nt_qq/nt_data` = NT 实例
   （可扫描其 nt_data）；不含 = legacy（不可扫描）。绝不因目录名是
   数字就放宽白名单——白名单结构校验（docs/06 §2）完全不变，只是
   nt_data 的物理位置随 NtRel 拼接。
3. Windows 实例的识别目录名即 QQ 号，但仍以「含 nt_qq/nt_data」为
   实例判定的**必要条件**——纯数字目录不是实例。
4. 未知布局（generic）行为不变：全拒绝。
5. 根候选探测只读（读注册表/环境变量/目录列表），零写入。

## 5. 测试策略

- **linux 容器覆盖 Windows 逻辑**：spec 是数据，nt 包测试直接换
  `currentSpec`，构造 Windows 布局 fixture（`<根>/10001/nt_qq/nt_data/…`
  + 顶层 Msg3.0.db）断言：Detect 命中 nt、InstanceDirs 返回 QQ 号实例、
  Identify 返回目录名、Discover 的 ntData 路径带 nt_qq、Residues 排除
  nt_qq 且计入 Msg3.0.db。
- **legacy 测试**：纯数字目录 + msg3.0.db → Detect 返回 legacy 且
  ScanCapable=false；混存根 → nt 胜出（现有测试保留并强化）；Residues
  统计整目录。
- **detect_test.go 更新**：TestDetectLegacyOnly 的期望从 generic 改为
  legacy（仍 incapable）。
- **红线测试**：legacy 根扫描必错；残留路径不可能通过 VerifyPath
  （AllowedRoots 只含 nt_data）。
- 现有全部测试（mac fixture）必须不回归——NtRel 零值语义 = mac 现状。

## 6. 待逆向确认清单（阻塞项标注）

| # | 问题 | 阻塞 |
|---|---|---|
| 1 | Windows nt_data 头部截断未见的目录（emoji-recv/marketface/*Temp 等可能量小或不存在） | 低（白名单结构驱动、存在性无关，fail-closed 天然兜底） |
| 2 | NTQQ 9.0.x 早期布局是否不同；根下是否可能混现 nt_qq_<hash> | 低（识别不依赖版本号；混现时 nt probe 两路都认） |
| 3 | TIM 与 QQ 是否同根同构（All Users\QQ 旧 Registry2.0.db 暗示多产品共存） | 无（TIM 记为非目标） |

已解决：mac 根路径（维持 docs 原值，Data/Documents 是误写）；mac 进程名
（QQ.app/Contents/MacOS/QQ，darwin 适配器现状正确）；mac 根下无数字目录
（legacy probe 不会误触）；`%APPDATA%\Tencent\QQ` 非数据根（binary +
全新安装 9.9.33 真机双证据，次级回退保留）；官方对旧版遗留采取「保留
+ 统计」不清理（Windows IDB 实证，与本工具策略一致）。
