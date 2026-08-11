# 03 — 清理规则与价值分级设计

> 本文件定义工具的**清理判定逻辑**。核心思路：
> ① 对齐 QQ 官方 3 天规则作为安全基线；
> ② **不做一刀切**——按目录类型、时间、文件类型、去重、大小做**价值分级**；
> ③ 诚实处理"按联系人/群组分级"的可行性边界（见 §5）。

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

**工具的默认阈值**：以 `259199` 秒为基线，但**每个目录类型可覆盖**（见 §4 分级表）。

---

## 2. 时间窗口模型

不用"3 天一刀切"，改用**分层时间窗**（每层独立可配置）：

| 层 | 时间窗 | 含义 | 默认动作 |
|---|---|---|---|
| L0 | mtime > 3 天 | 超过 QQ 自身门槛 | 至少列入"可清理"候选 |
| L1 | mtime > 30 天 | 近期未访问 | 建议清理（Thumb 类） |
| L2 | mtime > 90 天 | 一季度未访问 | 建议清理（Ori 类 + Thumb） |
| L3 | mtime > 180 天 | 半年未访问 | 强烈建议清理 |
| L4 | mtime > 365 天 | 一年未访问 | 归档级，默认列入 |
| L5 | 月目录年龄 > 12 个月 | 整个 `{YYYY-MM}` 归档 | 整月打包评估 |

**实现**：
```python
def time_tier(file_mtime: float, now: float) -> int:
    age = now - file_mtime
    if age > 365*86400: return 4   # L4
    if age > 180*86400: return 3   # L3
    if age >  90*86400: return 2   # L2
    if age >  30*86400: return 1   # L1
    if age > 259199:    return 0   # L0
    return -1                      # 太新，不碰
```

---

## 3. 目录类型优先级（Type Priority）

| 优先级 | 目录/子目录 | 理由 |
|---|---|---|
| P0（最可清） | `*OriTemp/` `*ThumbTemp/`（含 `BaseEmojiSyastems/ThumbTemp/*.zip`） | 下载中断残留，无引用价值 |
| P1 | `Emoji/emoji-recv/Thumb/`、`Pic|Video|.../Thumb/` | 缩略图，QQ 可从原图重建 |
| P2 | `Emoji/emoji-recv/Ori/` | 收到的表情原图，价值中等 |
| P3 | `Pic|Video|dataline/Ori/` | 原图/原视频，价值较高 |
| P4 | `Emoji/marketface/` | 表情包，QQ 按最近使用保留 |
| P5（最谨慎） | `Emoji/BaseEmojiSyastems/EmojiSystermResource/`、`File/`、`personal_emoji/` | 基础资源/传输文件/个人制作 |

**启发式指引（Thumb 是独立判定维度，与 biz 平级）**：
- 现象：Thumb 可重建（Ori 在则零损失）、占缓存大头（Pic 66%）、多尺寸冗余（`_0`/`_720`）
- 指引：可重建性（"是否缩略图"）应作为**与所属 biz 平级**的清理判定维度，而非仅占类型优先级一档；
  具体如何并入价值模型（如独立成维、或覆盖部分 type_score 职责）由开发者设计

---

## 4. 价值分级综合模型（核心）

**对每个候选文件计算 `value_score`（0~100），越低越可清**：

```
score = w1*type_score + w2*time_score + w3*redundancy_score + w4*size_score

type_score     (0~40):  由目录类型优先级映射（P0→0, P1→10, P2→20, P3→30, P4→35, P5→40）
time_score     (0~30):  由时间层映射（L0→0, L1→5, L2→10, L3→18, L4→25, L5→30）
redundancy     (0~20):  同 md5 已在 Ori 存在 → 缩略图冗余 +20 可清
                       同 md5 重复多次 → +10
size_score     (0~10):  大文件收益高（>10MB → 10, >1MB → 7, 其余 → 3）
```

**分级阈值**（默认，可配置）：
| 区间 | 标签 | 默认动作 |
|---|---|---|
| 0~30 | 🟢 可安全清理 | dry-run 报告 + `--force` 才删 |
| 31~55 | 🟡 建议清理 | 需用户确认 |
| 56~75 | 🟠 谨慎 | 默认保留，`--aggressive` 才列入 |
| 76~100 | 🔴 保留 | 永不自动清理 |

**示例**：
- `Pic/2021-04/Thumb/xxx_720.png`（7 年前缩略图，Ori 有同名 md5）→ type=10, time=30, redund=20 → 60 分 🟡~🟠
- `Pic/2026-07/Thumb/xxx_720.jpg`（1 月内缩略图）→ type=10, time=0, redund=20 → 30 分 🟢（边界）
- `Video/2023-01/Ori/xxx.mp4`（3 年前原视频 100MB）→ type=30, time=18, redund=0, size=10 → 58 🟠

---

## 5. ⚠️ "按联系人/群组分级"的可行性边界（重要）

**目标**：用户希望不同联系人/群组的记录价值不同，分开筛选。
**现实**：缓存文件按 `{md5}.{ext}` 命名，**文件系统层面不包含任何联系人/群组归属信息**；
归属信息只在加密 DB（`nt_db/*.db`）里。**本工具不解析 DB**——解析需运行时 dump enc_key（离线不可直接解析，但可通过社区工具如 `qq_key_for_mac` 运行时提取密钥后读取）。

**可行的代理方案**（推荐实现）：
1. **按月份/活跃度代理**：联系人/群组价值高 ⇒ 对应时段的文件多、mtime 新。
   工具按 `{YYYY-MM}` 目录聚合统计文件数与大小，**低活跃月份 = 低价值时段** → 优先清理。
   ```
   月活跃度 = 该月文件数 + 该月最后访问时间权重
   低活跃月（文件数 < 阈值 且 距今 > 6 个月）→ 整月降级为"建议清理"
   ```
2. **按文件类型代理**：群聊图片多（Thumb 占比高）、重要资料是 File。File 目录默认谨慎。
3. **按大小代理**：大文件（视频）删除收益大，但视频可能是有价值的记录 → 归入 🟠 谨慎层。

**诚实输出**：报告应明确说明"无法区分具体联系人/群组"，但提供**按月份/类型的价值视图**，
让用户基于自己的记忆做最终判断（dry-run 报告 + 交互确认天然支持这个流程）。

**未来方向（若需要真正的联系人维度）**：
- 运行时内存提取 enc_key（社区工具 `qq_key_for_mac`）→ 解密 DB → 读 `files_in_chat.db` 得到
  文件 md5 ↔ 会话归属 → 才能真正按联系人/群组分级。**这超出本工具范围**，作为可选扩展点记录。

---

## 6. 清理执行规则（结合 06 红线）

1. **只允许删白名单内路径**（见 01 §2.1 富媒体缓存 + \*Temp），配置/状态目录工具不清理
2. 默认 dry-run，生成完整报告（路径 + 大小 + 分级 + 原因）
3. `--force` 时：先备份（移动到用户指定备份目录或记录清单+校验和）→ 二次确认 → 执行
4. 执行前检查 QQ 是否运行（`pgrep -f "QQ.app"`），运行中拒绝
5. 每删一个文件记录日志（便于恢复与审计）

---

## 7. 配置设计（默认值汇总）

```yaml
# config.yaml（工具默认，可被 --config 覆盖）
default_threshold_seconds: 259199        # 对齐 QQ 3 天
time_tiers: {30d: 1, 90d: 2, 180d: 3, 365d: 4}
archive_month_older_than_months: 12
classify:
  clean_temp: true                        # *Temp 直接列入
  clean_thumb: true                       # Thumb 参与分级
  clean_ori: false                        # Ori 默认只报告不清
  clean_base_emoji: false
  clean_marketface: false
  clean_personal_emoji: false
  clean_file: false
score_thresholds: {safe: 30, suggest: 55, caution: 75}
min_file_size_bytes: 0                    # 只统计大于此值的文件
skip_dirs: [mmkv, msf, OnlineStatus, UnitedConfig, config, log, avatar, ...]
```
