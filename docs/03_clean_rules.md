# 03 — 清理规则与可清性政策设计

> 本文件定义工具的**清理判定逻辑**。核心思路：
> ① 对齐 QQ 官方 3 天规则作为扫描基线；
> ② **不做一刀切**——按类别门控（clean_* 开关）+ 用户筛选器决定清理什么；
> ③ 诚实处理"按联系人/群组分组"的可行性边界（见 §5）。
>
> **无打分/分级模型**：工具不计算 value_score，不产出 safe/suggest/caution/keep
> 档位。可清性 = 白名单结构校验 + 类别门控；「选什么清理什么」由用户的
> 命名筛选器显式表达。

---

## 1. QQ 官方清理逻辑（逆向结论 = 默认基线）

```
for each file in nt_data 扫描范围:
    if (mode_flag == 0) OR ((now - file_mtime) > 259199):   # 259199 秒 ≈ 3 天
        计入可清理
```
- **3 天阈值**：`0x3F47F = 259199 秒`（按秒比较，非日历天）
- **mode_flag==0**：QQ 的"清空"模式，全部可清（工具不实现此模式，太危险）
- **\*Temp 目录**：`OriTemp`/`ThumbTemp` 是下载中断/未完成残留，QQ 单独优先清理
- **引用判定**：QQ 解析加密 DB 判断"文件是否仍被消息引用"→ 工具不解析 DB（解析需运行时 dump enc_key），只能用启发式

**官方「缓存文件」category（IDB 实证，2026-08-15，mac CacheScannerWorker）**：
官方清理器对「缓存文件」与「普通缓存」是**两个独立扫描入口**，共用同一
259199s(3天)/mode_flag 统计逻辑（同一 worker 类）——差别只在路径集合：

| 入口 | 路径集合 |
|---|---|
| `StartScanCacheFiles`（UI「缓存文件」category；GetFileCacheInfo type==3） | 富媒体 biz {Pic,Video,Ptt,File,Emoji,dataline,FilterVideo} × `{OriTemp,ThumbTemp}` + biz29 = `flashfransfer/{upload_temp,download_temp,thumb}` |
| `StartScanNormalFiles`（普通缓存） | 同 biz 集合 × `{Ori,Thumb}`（dataline 用 `.thumb`） |

- `SilentCache` 仅是**扫描标记**（路径名含 "SilentCache" 时打静默缓存
  标志，sub_22A454E），不是独立的目录集合——无额外路径注册。
- ⇒ 官方「缓存文件」的核心就是我们工具里的 **\*Temp 集合（clean_temp 首选
  清理）+ flashfransfer 三个中转子目录**，方向一致（工具已把 flashfransfer
  挂 clean_temp 门控，docs/01 §2.3）。
- 注意命名碰撞：本工具给 BaseEmoji 资源的展示标签也叫「缓存文件」
  （自有分类），与官方同名 category（≈ 下载中断残留）不是一回事。

**工具对齐方式**：扫描默认跳过 mtime 距今 < 3 天的文件（`MinAgeDays` 默认 3，
`internal/rules/config.go` 的 `DefaultMinAgeDays`）。更细的时间选择交给筛选器
（`age >= 90`、`month <= 2025-12` 等表达式）。

---

## 2. 时间维度（扫描基线 + 筛选器字段）

- **扫描基线**：`MinAgeDays`（默认 3 天，对齐 QQ 基线）。太新的文件连列表都不进。
- **时间选择由筛选器表达**：`age`（N 天前）、`month`（YYYY-MM，支持范围比较）是
  筛选表达式的一等字段；内置筛选器提供「缩略图·90天前」「一年前」等常用档。
  内置时间预设（「XX前」）按**月份边界**表达（`month lte YYYY-MM`，种入时
  按当日计算），与左栏月份树的勾选天然联动——同月文件整体进入/排除。
- 月目录 `{YYYY-MM}` 是时间聚合的天然单位（左栏月份树按它聚合）。

工具**不再维护** L0–L5 时间层与"归档级"概念——那些是打分模型的残留。

---

## 3. 类别与 reason 标签

按目录类型整理的风险顺序（逆向结论），**不参与打分**，仅决定两件事：
① 各类目的默认门控（§6 配置）；② 该文件在报告里的 reason 说明标签。

| 风险 | 目录/子目录 | 默认门控 | reason 标签 |
|---|---|---|---|
| 最低 | `*OriTemp/` `*ThumbTemp/`（含 `BaseEmojiSyastems/ThumbTemp/*.zip`） | clean_temp=true | 下载中断残留 |
| 最低 | `dataline/.tmp/`（NFC 未完成拷贝传输残留，实测布局） | clean_dataline_tmp=false（高级） | 传输残留 |
| 最低 | `log/` `log-cache/`（运行日志，QQ 自动重建） | clean_log=false（高级） | 运行日志 |
| 最低 | `avatar/`（头像缓存，重新拉取） | clean_avatar=false（高级） | 头像缓存 |
| 低 | `Emoji/emoji-recv/Thumb/`、`Pic|Video|.../Thumb/` | clean_thumb=true | 缩略图 |
| 中 | `Emoji/emoji-recv/Ori/` | clean_ori=false | 原图/原文件 |
| 中 | `Pic|Video|dataline/Ori/` | clean_ori=false | 原图/原文件 |
| 较高 | `Emoji/marketface/` | clean_marketface=false | 表情包 |
| 高 | `Emoji/BaseEmojiSyastems/EmojiSystermResource/` | clean_base_emoji=false | 缓存文件 |
| 高 | `File/` | clean_file=false | 原图/原文件 |
| 高 | `personal_emoji/` | clean_personal_emoji=false | 个人表情 |

**缩略图是独立判定维度，与 biz 平级**：Thumb 可重建（Ori 在则零损失）、占缓存
大头、跨 biz 普遍存在——索引与筛选都必须支持按「是否缩略图」独立聚合。

---

## 4. 可清性判定模型

**没有任何评分**。一个文件能否被清理，只由以下环节决定（全部在 Go 侧）：

1. **扫描**：按白名单目录遍历（docs/06 §2），黑名单目录/db 文件双重过滤；
   跳过 mtime < MinAgeDays 的文件。
2. **内容哈希二次扫描**（只读）：为「与其它文件字节数完全相同」的文件计算
   SHA-256 —— 字节级重复的判定依据。同内容 ⇒ 同大小，所以大小唯一的文件
   跳过不读（I/O 最小化）。
3. **选择**：用户用命名筛选器（条件 + 排序 + take/drop）勾选出要清理的文件。
4. **执行前逐文件重验**（clean 层，docs/06 §5b）：
   - 路径在允许的 nt_data 根目录内（防穿越）；
   - 命中白名单（类别门控 `clean_*` 打开才可能被删；GUI 模式全部打开，
     选择权在用户；CLI 按 config 保守默认）；
   - 不命中硬黑名单（状态目录/db 后缀）。

**reason 标签**（`rules.Reason`）只做展示说明，**不影响可清性**。标签有：
「下载中断残留」「传输残留」「运行日志」「缩略图」「表情包」「个人表情」
「原图/原文件」「原图仍在」「有缩略图」「重复出现」「缓存文件」——前端
悬浮 tooltip 解释每个标签的含义（docs/07 §4.4）。

**关联情况严格区分三种**（同名 ≠ 同内容，同内容也可能不同名）：

| 标签 | 含义 | 依据 |
|---|---|---|
| 重复出现 | 字节级内容相同的其它副本存在（QQ 只按目录去重，跨月份/目录/账号会各存一份） | SHA-256 内容哈希组（≥2 份） |
| 原图仍在 | 这个缩略图有对应的原文件，可重建 | 文件名 md5 配对（Ori 存在） |
| 有缩略图 | 这个原文件有对应的缩略图缓存 | 文件名 md5 配对（Thumb 存在） |

原文件绝不再因「同名缩略图」被误标「重复出现」——配对与重复是两套独立索引。

---

## 5. ⚠️ "按联系人/群组分组"的可行性边界（重要）

**目标**：用户希望不同联系人/群组的记录价值不同，分开筛选。
**现实**：缓存文件按 `{md5}.{ext}` 命名，**文件系统层面不包含任何联系人/群组归属信息**；
归属信息只在加密 DB（`nt_db/*.db`）里。**本工具不解析 DB**——解析需运行时 dump enc_key（离线不可直接解析，但可通过社区工具如 `qq_key_for_mac` 运行时提取密钥后读取）。

**可行的代理方案**（推荐实现）：
1. **按月份/活跃度代理**：联系人/群组价值高 ⇒ 对应时段的文件多、mtime 新。
   工具按 `{YYYY-MM}` 目录聚合统计文件数与大小（左栏月份树），用户按自己的
   记忆筛选月份。
2. **按文件类型代理**：群聊图片多（Thumb 占比高）、重要资料是 File。
   File 默认只报告不清理（clean_file=false）。
3. **按大小代理**：大文件（视频）删除收益大，用大小排序/筛选（`size gt 100M`）
   自行圈定。

**诚实输出**：报告明确说明"无法区分具体联系人/群组"，但提供**按月份/类型的
聚合视图**，让用户基于自己的记忆做最终判断（dry-run 报告 + 交互确认天然支持
这个流程）。

**未来方向（若需要真正的联系人维度）**：
- 运行时内存提取 enc_key（社区工具 `qq_key_for_mac`）→ 解密 DB → 读 `files_in_chat.db` 得到
  文件 md5 ↔ 会话归属 → 才能真正按联系人/群组筛选。**这超出本工具范围**，作为可选扩展点记录。

---

## 6. 配置设计（默认值汇总）

```yaml
# config.yaml（工具默认，可被 --config 覆盖；完整注释版见 config.example.yaml）
clean_temp: true                        # *Temp 直接可清
clean_thumb: true                       # 缩略图可清
clean_ori: false                        # 原图/原文件默认只报告不清
clean_base_emoji: false
clean_marketface: false
clean_personal_emoji: false
clean_file: false
clean_log: false                        # 高级 opt-in：运行日志
clean_dataline_tmp: false               # 高级 opt-in：传输缓存（NFC 残留）
clean_avatar: false                     # 高级 opt-in：头像缓存
min_file_size_bytes: 0                  # 只统计大于此值的文件
skip_dirs: [mmkv, msf, OnlineStatus, UnitedConfig, config]
backup_dir: ""                          # 移动备份目录（仅当显式选择移动时使用，空 = 直接删除）
```

- **GUI 模式**：普通类别门控全部放开（选什么清理由用户的筛选器决定）；
  **高级 opt-in 三类**（传输缓存/日志/头像）默认**关闭**——关闭时扫描
  不入索引、清理拒绝，在设置→高级勾选后才覆盖（变更自动重扫）。结构性
  红线（nt_db/db 文件/mmkv 等黑名单、路径穿越）照常强制。
- **CLI 模式**：按 config 保守默认（如上表）；扫描报告全部结构合法文件，
  `clean` 执行时每个文件逐条重验（门控关闭的只报告不清）。
