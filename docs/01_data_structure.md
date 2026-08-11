# 01 — QQ 缓存目录结构全解

> 本文件是清理工具**遍历与分类**的基础。所有路径均相对 QQ 数据根目录（下称 `<QQ>`）。
> `<QQ>` 由工具**自动探测或 `--root` 指定**；默认位置：macOS
> `~/Library/Containers/com.tencent.qq/Data/Library/Application Support/QQ`，Windows 待探测
> `%APPDATA%\Tencent\QQ` 等候选（见 `04_tool_design.md` §4.1）。

---

## 1. 顶层结构

```
<QQ>/
├── global/                      # 全局（跨账号）
│   ├── nt_data/                 # 全局缓存/配置
│   │   ├── Login/               # .<qq号> 空文件 = 登录过的账号标记（明文）
│   │   ├── mmkv/                # mmkv.default 等（明文，含账号路径映射）
│   │   ├── UnitedConfig/        # 全局配置
│   │   └── (Emoji/Log/msf/Skin/... 全局缓存)
│   ├── nt_db/                   # 全局加密库（login.db），读取需 dump enc_key，工具不清理
│   └── nt_temp/
├── nt_qq_<32hex>/               # ★ 每账号一个实例目录
│   ├── nt_data/                 # ★ 缓存主体（清理目标）
│   ├── nt_db/                   # 账号加密库，读取需 dump enc_key，工具不清理
│   └── nt_temp/                 # 临时文件，可清
└── <数字>/                      # 旧版明文目录（登录前旧格式）
```

**实例目录命名**：`nt_qq_` + MD5(MD5(uid) + "nt_kernel")。uid 是内部标识（≠ QQ 号），
**不要试图从目录名反推 QQ 号**——用 `02_account_identification.md` 的明文方案。

---

## 2. nt_data 缓存目录清单（工具遍历范围）

### 2.1 富媒体缓存（主要清理目标）

> 参考值列来自开发机样例（见 05），仅示意数量级，**非通用数据**。

| 目录 | 内容类型 | 年月结构 | 子目录 | 参考值* |
|---|---|---|---|---|
| `Pic/` | 图片 | `{YYYY-MM}/` | `Ori` `Thumb` | ~44G |
| `Video/` | 视频 | `{YYYY-MM}/` | `Ori` `Thumb` | ~1G |
| `Ptt/` | 语音 | `{YYYY-MM}/` | `Ori` `OriTemp` | ~7M |
| `File/` | 文件传输 | **无年月** | `file_assistant` `Thumb` `ThumbTemp` | ~200M |
| `dataline/` | 数据线 | `{YYYY-MM}/` | `Ori` `Thumb` | ~50M |
| `Emoji/` | 表情 | 混合 | 见 §3 | ~18G |

### 2.2 标准路径模板（来自 QQ 内部路径拼装逻辑，已实证）

```
{nt_data}/{biz}/{YYYY-MM}/{Ori|Thumb|OriTemp|ThumbTemp}/{md5}[_{size}].{ext}
```

规则：
- **biz 目录**：`Pic`/`Video`/`Ptt`/`File`/`Emoji`/`dataline` 等（见 §2.3）
- **年月目录**：`{YYYY}-{MM}`（如 `2024-09`）；月份个位补零；跨度为该账号的活跃起止期
- **Ori/Thumb 选择**：原始图 → `Ori`；缩略图 → `Thumb`；下载中/中断 → `*Temp`
- **文件名**：`{md5}` 或 `{md5}_{size}`（`down_type==2` 时带尺寸后缀）
- **扩展名**：图片 `.jpg`、视频 `.mp4`、缩略图 `.png`（也见 `.gif` 等）
- **File（biz=5）特例**：**不拼年月**，直接 `File/{Ori|Thumb|OriTemp|ThumbTemp}`
- **emoji-recv 特例**：`Emoji/emoji-recv/{YYYY-MM}/Thumb/...`（收到的表情）

**Ori/Thumb 是贯穿 biz 的公共维度（现象与指引）**：
- 现象：所有富媒体 biz（Pic/Video/File/dataline/emoji-recv）都含 `Thumb` 子目录；同一张图 = 1 份 Ori + 多份 Thumb（`_0`/`_720`）；**Thumb 可重建**（Ori 在则删除零损失）
- 指引：文件分类应将"是否缩略图（sub=Thumb）"作为**与 biz 平级**的一级维度；缩略图删减应能**跨 biz 统一**评估与操作（具体设计由开发者定）

### 2.3 biz 目录名映射（内部业务码 → 目录名）

| 内部码 | 目录名 | 说明 |
|---|---|---|
| 2 | `Pic` | 图片 |
| 3 | `Video` | 视频 |
| 4 | `Ptt` | 语音 |
| 5 | `File` | 文件（无年月） |
| 6 | `Emoji` | 表情 |
| 12 | `dataline` | 数据线 |
| 31 | `FilterVideo` | 过滤视频 |
| 其他 | `GroupAnnouncement/avatar/Log/msf/OnlineStatus/PrivilegeIcon/PhotoWall/Qzone/...` | 见下 |

### 2.4 其他 nt_data 目录（非富媒体缓存）

| 目录 | 内容 | 清理建议 |
|---|---|---|
| `log/` `log-cache/` | 运行日志 | ★★ 可按龄清理（低价值） |
| `avatar/` | 头像缓存 | ★ 只清超龄 Thumb |
| `msf/` | 消息服务框架状态 | ❌ 工具不清理（状态数据） |
| `mmkv/` | 键值配置（**明文可读**，含账号映射） | ❌ 工具不清理 |
| `OnlineStatus/` | 在线状态 | ❌ 工具不清理 |
| `UnitedConfig/` | 配置（含 QQ 号子目录） | ❌ 工具不清理 |
| `bc_09.db` 等 .db | 明文 SQLite 库（**可读**） | ❌ 工具不清理 |
| `wmpf/ ams/ flashfransfer/` | 其他 | 视内容，flashfransfer 可能为空 |

---

## 3. Emoji 五子类详解（常为缓存大头，需分细类处理）

```
Emoji/
├── BaseEmojiSyastems/       # 基础表情资源（注意拼写 Syastems）
│   ├── EmojiSystermResource/{表情字符}/png/...   # 354 个表情目录，正式资源（QQ 可重下）
│   └── ThumbTemp/{id}_{base|advance}.zip          # 表情包下载暂存 zip（可安全清）
├── emoji-recv/              # 收到的表情，{YYYY-MM}/Thumb/{md5}_{size}.{ext}
├── marketface/              # 表情包，按表情包 ID 分目录（QQ 按"最近使用"保留）
├── emoji-related/           # 相关表情
└── personal_emoji/          # 个人表情，{Ori|Thumb}（无年月）
```

**清理建议**：
- `BaseEmojiSyastems/ThumbTemp/*.zip` = 下载中间产物，**可安全清理**
- `BaseEmojiSyastems/EmojiSystermResource` = 基础表情资源，**谨慎**（QQ 有接口可整体重下）
- `emoji-recv` = 收到的表情，按年月超龄清理（同 Pic 规则）
- `marketface` = 表情包，QQ 按"最近使用"保留 → 工具按 mtime 分级
- `personal_emoji` = 个人表情，**用户主动制作，默认不清除**

---

## 4. 文件名与内容判定（启发式）

### 4.1 命名模式
- `{32位hex md5}.{ext}` → 原图/文件
- `{32位hex md5}_{size}.{ext}` → 缩略图（size 如 `0`、`720`、`1080`）
- 同 md5 不同 size = 同一文件的多尺寸缩略图

### 4.2 扩展名分布（样例）
> 开发机 Pic 目录的扩展名统计（见 05），仅示意常见类型：`.png` / `.jpg` / `.gif`（及少量 `webp` 等）

### 4.3 价值判定启发式（不解析 DB）
QQ 官方用 DB 引用判断"这个文件是否还被消息引用"。本工具**不解析加密 DB**（解析需运行时 dump enc_key），改用以下启发式近似：
- **\*Temp 目录** → 下载中断/未完成 → 最高可清理优先级
- **Thumb 目录** → 缩略图，QQ 可从 Ori 重建 → 高可清理优先级
- **Ori 目录** → 原图，价值最高 → 低清理优先级
- **老月份目录**（如 > 12 个月）→ 大概率不再被查看 → 按价值分级
- **重复 md5**（Ori 与 Thumb 同 md5）→ 缩略图可清

---

## 5. 清理工具不处理清单（硬性行为红线）

> **注意**：本清单指**清理工具不删除/不修改**这些文件（行为红线），**不代表不可读**。
> 加密库（`nt_db/*.db`）可用运行时 dump enc_key 等特殊手段读取；`mmkv/`、`UnitedConfig/`、`bc_09.db` 本就是明文可读。

| 路径 | 读取方式 | 工具行为 |
|---|---|---|
| `<QQ>/nt_qq_*/nt_db/` | 需 dump enc_key（特殊手段） | 不清理 |
| `<QQ>/global/nt_db/` | 需 dump enc_key（特殊手段） | 不清理 |
| 任何 `*.db` / `*.db-wal` / `*.db-shm` / `*.db-first.material` / `*.db-last.material` | 加密库需 dump enc_key；`bc_09.db` 等为明文 | 不清理 |
| `mmkv/` `msf/` `OnlineStatus/` `UnitedConfig/` `config/` | mmkv/UnitedConfig 明文可读 | 不清理（配置/状态） |
| `nt_data/bc_09.db` 等明文库 | 明文可读（标准 SQLite） | 不清理 |
| 账号目录根、`nt_temp` 之外的系统文件 | — | 不清理 |

---

## 6. 目录发现算法（正向）

```python
def discover_instances(qq_root):
    """返回 [{hash, qq_num, nt_data_path, nt_temp_path, nt_db_path}]"""
    instances = []
    for d in os.listdir(qq_root):
        if not d.startswith("nt_qq_"):
            continue
        inst = {
            "hash": d[len("nt_qq_"):],
            "nt_data": os.path.join(qq_root, d, "nt_data"),
            "nt_temp": os.path.join(qq_root, d, "nt_temp"),
            "nt_db":   os.path.join(qq_root, d, "nt_db"),
        }
        inst["qq_num"] = identify_account(qq_root, inst["hash"])  # 见 02
        instances.append(inst)
    return instances
```
