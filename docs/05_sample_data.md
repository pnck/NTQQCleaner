# 05 — 开发机样例数据（测试与验证参考）

> ⚠️ **本文件数据来自一台开发机的实际 QQ 账号，仅供开发测试与验证基准使用，非通用数据。**
> 实际用户环境中的账号数、大小、月份范围、文件数量各不相同；工具实现**不得依赖**本文件中的任何具体值。
> 数据采集：macOS，QQ 6.9.99，截至 2026-08-09。

---

## 1. 账号清单（三源交叉验证）

| 账号目录 | QQ 号 | 状态 | nt_data 最新 Pic 月 | 规模 |
|---|---|---|---|---|
| `nt_qq_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb1` | **10001** | 当前/最近 | 2026-08 | ~63G |
| `nt_qq_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb2` | **10002** | 旧 | 无 Pic（或无） | ~240M |

验证来源：
- `global/nt_data/mmkv/mmkv.default`：两条 `nt_qq_<hash>...<qq号>_flash` 路径 key
- `nt_qq_61f0.../nt_data/UnitedConfig/10001/`（子目录名 = QQ 号）
- `global/nt_data/Login/.10001`、`.10002`（空文件标记）

---

## 2. 当前账号（10001）nt_data 大小分布

```
总计 ~63G
Pic      44G    ← 最大目标
Emoji    18G
Video    1.1G
log      382M
File     217M
avatar   135M
dataline 50M
Ptt      6.7M
Qzone    5.4M
PrivilegeIcon 2.1M
OnlineStatus  1.5M
log-cache     472K
PhotoWall     456K
UnitedConfig  236K
mmkv     64K
msf      24K
bc_09.db 12K   （明文 SQLite 库，可读，工具不清理）
wmpf/ams/flashfransfer  0B（空）
```

**关键观察**：
- Pic + Emoji = 62G，占总量的 98% → 清理工具主战场
- `nt_temp` 222M、`nt_db` 2.8G（nt_db 工具不清理；读取需 dump enc_key）
- **Thumb 总量是 Ori 的 ~2 倍**（29.1G vs 14.9G）→ **缩略图是清理大头**（见 §3.4）

### 2.1 去重与膨胀实证（Pic Ori vs Thumb 全量拆解）

| | 大小 | 文件数 |
|---|---|---|
| Ori（原图） | 14.9 GB | 46,891 |
| Thumb（缩略图） | **29.1 GB** | **93,749** |

- **md5 去重有效**：同内容同目录只存一份（Pic 46,891 个 Ori = 46,891 张不同原图）
- **膨胀来自多尺寸缩略图**：同一张原图生成 `_0` + `_720` 等多张（实测同 md5 计 2 次），每张独立文件
- **Thumb 占 Pic 66%** → 删 Thumb 零数据损失（Ori 仍在，QQ 可从 Ori 重建）
- **结论：不做内容去重（无冗余），主打缩略图 + 超龄清理**

---

## 3. Pic 结构细节

### 3.1 月目录（53 个月，2021-04 ~ 2026-08）
```
2021-04 2021-06 2021-07 2021-11 2022-03 2022-07 2022-10 2022-11 2022-12
2023-01 ... 2023-12  2024-01 ... 2024-12  2025-01 ... 2025-12  2026-01 ... 2026-08
```
- 每月结构：`{YYYY-MM}/{Ori|Thumb}/`
- **低活跃月份示例**：2021-04/06/07、2022-03/07/10/11/12 等早期月份 → 归档级候选

### 3.2 文件命名样本
```
Pic/2024-09/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa09_720.png
Pic/2024-09/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa10_0.jpg
Pic/2024-09/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa11_720.png
Pic/2024-09/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa12_0.jpg
```
- `{32hex md5}_{size}.{ext}`，size ∈ {0, 720, ...}
- 同 md5 多个 size = 同一图多尺寸缩略图（`_0` 与 `_720` 成对）

### 3.3 扩展名统计（Pic 全量）
```
png  70491
jpg  59229
gif   7342
```
- 文件总数：**140,585**（Pic 一个目录）

### 3.4 多尺寸缩略图（膨胀来源）
- 同一 `md5` 的 `_0` / `_720` 成对出现（2024-09 Thumb 抽样：228 个 `_0` + 214 个 `_720`）
- 即：**1 张原图 = 1 份 Ori + 2 份 Thumb**（内容不同、md5 不同，不违反去重）

---

## 4. Emoji 结构

```
Emoji/
├── BaseEmojiSyastems/          # 基础表情 ~133M
│   ├── EmojiSystermResource/{表情字符}/png/   # 354 个表情目录
│   └── ThumbTemp/{id}_{base|advance}.zip      # 下载暂存
├── emoji-recv/                 # 收到的表情, {YYYY-MM}/Thumb/{md5}_{size}.{ext}
├── marketface/                 # 表情包（按包 ID 分目录, 408 个）
├── emoji-related/
└── personal_emoji/             # 个人表情 {Ori|Thumb}
```
- `emoji-recv` 样本：`Emoji/emoji-recv/2024-09/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa13_720.png`

### 4.1 Emoji 子目录实测拆解（18G）

| 子目录 | 大小 | 文件数 | 说明 |
|---|---|---|---|
| **emoji-recv** | **18.2 GB** | **72,593** | 收到的表情（占 99%）；46 个月目录（2022-03 起） |
| BaseEmojiSyastems | 133 MB | 1,011 | 系统基础表情（354 个） |
| personal_emoji | 120 MB | 933 | 个人制作表情 |
| marketface | 49 MB | 1,056 | 表情包（410 个包子目录） |
| emoji-related | 0 | 0 | 空 |

- **emoji-recv 有去重**（md5 命名，同表情只存一次）；18.2G = 5 年累积的 **72,593 个不同表情**
- 与 Pic 完全同构：去重有效但挡不住新内容累积；emoji-recv 以 Thumb 为主（可重建）
- marketface 按包 ID 分目录（410 个），包内去重、包间独立
- BaseEmojiSyastems 为系统基础表情（QQ 可 `delAllBaseEmojiResource` 整体重下）

---

## 5. 其他目录结构

### 5.1 File（无年月）
```
File/
├── file_assistant/
├── Thumb/
└── ThumbTemp/
```

### 5.2 旧账号（10002）nt_data（~240M）
```
Emoji 182M / Pic 51M / avatar 2.6M / log 2.1M / OnlineStatus 1.4M / PrivilegeIcon 592K
```
- 旧账号小得多，且 UnitedConfig 无 QQ 号子目录（只有 000）→ 需 mmkv 来源识别

### 5.3 global 目录
```
global/nt_data: common Log msf search VasUpdateSystem Emoji Login onlineStatus
                Skin WeatherBgCache Font mmkv PokeFace UnitedConfig
global/nt_db:   login.db 等（加密, 读取需 dump enc_key, 工具不清理）
```

---

## 6. 测试夹具建议（从本数据派生）

在 `tests/fixtures/qq/` 构造小型假目录：
```
qq/
├── global/nt_data/mmkv/mmkv.default      # 含两条 nt_qq↔QQ 映射
├── global/nt_data/Login/.10001
├── nt_qq_aaaa.../nt_data/
│   ├── Pic/2023-01/Ori/{md5}.jpg          # 老 Ori
│   ├── Pic/2023-01/Thumb/{md5}_720.jpg    # 老 Thumb（同 md5）
│   ├── Pic/2026-07/Thumb/{md5}_720.jpg    # 新 Thumb
│   ├── Pic/2026-07/OriTemp/{md5}.tmp      # 中断残留
│   ├── Emoji/emoji-recv/2024-05/Thumb/{md5}_720.png
│   ├── File/Thumb/{md5}.png
│   └── mmkv/                              # 应被 skip
└── nt_qq_bbbb.../nt_data/Pic/2022-01/Thumb/{md5}_720.jpg
```
用 `touch -d` 设置不同 mtime，验证各时间层与分级结果。
