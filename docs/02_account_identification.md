# 02 — 账号识别方案（`nt_qq_<hash>` ↔ QQ 号）

> 账号目录名是 `nt_qq_` + MD5(MD5(uid)+"nt_kernel")，uid 是**内部标识（≠ QQ 号）**，
> 无法从目录名反推。但存在**三个完全明文的来源**可直接拿到映射，无需解密任何数据库。
> 本方案经开发机双账号交叉验证，逻辑通用。

---

## 1. 结论速查（示例）

| 账号目录 hash | QQ 号 | 可用来源 |
|---|---|---|
| `nt_qq_<32hex>`（示例 A） | **QQ <号码>** | mmkv.default ✓ + UnitedConfig ✓ + Login ✓ |
| `nt_qq_<32hex>`（示例 B） | **QQ <号码>** | mmkv.default ✓ + Login ✓ |

---

## 2. 来源一（最可靠）：`global/nt_data/mmkv/mmkv.default`

**路径**：`<QQ>/global/nt_data/mmkv/mmkv.default`

**原理**：这是一个 mmkv 键值文件（二进制但含明文 UTF-8 字符串）。其中 flash_transfer
相关的 key 把**账号目录路径**和 **QQ 号**拼在一起，可直接配对：

```
.../nt_qq_<32hex>/nt_data/flashfransfer+<QQ号>_flash_transfer_document_cache_key
.../nt_qq_<32hex>/nt_data/flashfransfer+<QQ号>_flash_file_download_dir_key
```

**实现（Go）**：
```go
func IdentifyFromMmkv(qqRoot, instanceHash string) (string, error) {
    p := filepath.Join(qqRoot, "global", "nt_data", "mmkv", "mmkv.default")
    data, err := os.ReadFile(p)
    if err != nil {
        return "", err
    }
    // 模式: nt_qq_<hash> ... <5~12位数字>_flash
    re := regexp.MustCompile(`nt_qq_` + regexp.QuoteMeta(instanceHash) + `[^0-9]*?(\d{5,12})_flash`)
    m := re.FindStringSubmatch(string(data))
    if len(m) == 2 {
        return m[1], nil
    }
    return "", nil
}
```

> 注意：`nt_qq_<hash>` 与数字之间可能有路径分隔符/其他字符，用 `[^0-9]*?` 宽松匹配；
> `_flash` 后缀保证数字是 QQ 号而不是其他数字。

---

## 3. 来源二：`nt_qq_*/nt_data/UnitedConfig/` 子目录名

**路径**：`<QQ>/nt_qq_<hash>/nt_data/UnitedConfig/`

**原理**：目录下通常有 `000`（默认配置）和以 **QQ 号命名**的子目录（每账号一个）：
```
UnitedConfig/
├── 000/
└── <QQ号>/            # ← 这就是 QQ 号
    ├── 10001/
    ├── 10002/
    ...
```

**实现（Go）**：
```go
func IdentifyFromUnitedConfig(ntData string) (string, error) {
    uc := filepath.Join(ntData, "UnitedConfig")
    entries, err := os.ReadDir(uc)
    if err != nil {
        return "", err
    }
    for _, e := range entries {
        n := e.Name()
        if n != "000" && len(n) >= 5 {
            if _, err := strconv.Atoi(n); err == nil {
                return n, nil
            }
        }
    }
    return "", nil
}
```

> 局限：某些账号可能只有 `000`（无 QQ 号子目录）→ 用来源一/三兜底。

---

## 4. 来源三：`global/nt_data/Login/.<qq号>` 标记文件

**路径**：`<QQ>/global/nt_data/Login/`

**原理**：每个登录过的账号有一个 `.` 开头的**空文件**，文件名即 QQ 号：
```
Login/
├── .<QQ号1>     # 0 字节
└── .<QQ号2>     # 0 字节
```

**实现（Go）**：
```go
func ListLoggedAccounts(qqRoot string) ([]string, error) {
    lg := filepath.Join(qqRoot, "global", "nt_data", "Login")
    entries, err := os.ReadDir(lg)
    if err != nil {
        return nil, err
    }
    var out []string
    for _, e := range entries {
        n := e.Name()
        if strings.HasPrefix(n, ".") && len(n) > 1 {
            if _, err := strconv.Atoi(n[1:]); err == nil {
                out = append(out, n[1:])
            }
        }
    }
    return out, nil
}
```

> 局限：只列出"登录过哪些号"，不直接关联到目录 → 用于**交叉验证**。

---

## 5. 推荐综合算法

**实现（Go）**：
```go
// IdentifyAccount 三源综合：mmkv 最可靠，UnitedConfig 兜底，Login 交叉验证
func IdentifyAccount(qqRoot, instanceHash, ntData string) string {
    qq, _ := IdentifyFromMmkv(qqRoot, instanceHash)
    if qq == "" {
        qq, _ = IdentifyFromUnitedConfig(ntData)
    }
    if qq == "" {
        qq = "unknown"
    }
    // 交叉验证（可选）：ListLoggedAccounts(qqRoot) 含 qq 则确认
    return qq
}
```

---

## 6. 输出形式（工具展示）

```
账号 1: nt_qq_<32hex>  →  QQ <号码>   （最近使用）
账号 2: nt_qq_<32hex>  →  QQ <号码>   （旧账号）
账号 3: nt_qq_<32hex>  →  unknown     （未识别，仅按时间排序）
```

- 账号新旧判定：用 `nt_data` 目录 mtime 或 Pic 月目录最新月份排序
- 每个账号独立统计与清理，**禁止跨账号混淆**

## 7. 已知但未采纳：login.db 固定密钥离线解密

逆向实测结论（2026-08-15，双平台实测成功，完整记录在逆向侧 docs/10 §14）：

- `nt_db/login.db` 可用**固定密钥**离线解密（`BD156D6710D54D8782F4`，
  所有账号/设备相同，无需 hook）；平台差异：mac `hmac=HMAC_SHA512` /
  Windows `hmac=HMAC_SHA1`（其余参数同：剥 1024 字节头、page=4096、
  iter=4000、kdf=PBKDF2_HMAC_SHA512）。
- `login_table` 直接存 uid ↔ QQ号 ↔ 账号目录绝对路径映射（并含昵称）。
  实测全表字段中**三源拿不到**的仅有：A1 登录票据（1004 BLOB）、头像
  URL（qlogo.cn）、头像文件路径（1010/1011，mmkv 已有完整路径，非
  增量）——均对清理工具无价值；密钥硬编码于 `LoginRecordManager::Init`
  （qmemcpy 写死），版本升级换值即失效。价值边际为零（逆向侧实测
  结论，2026-08-15）。

**本工具不采纳**，理由：

1. **痛点不存在（常见情况）**：§2-§4 的三源明文识别已覆盖账号↔QQ号
   映射，报告层已展示 QQ号；真正盲区是「三源全缺」的罕见账号，为
   此引入 DB 解析不成比例。
2. **架构成本**：SQLCipher 解析需 cgo（go-sqlcipher）→ 破坏「容器内
   交叉编译 Windows exe（CGO_ENABLED=0）」的核心构建属性；捆绑
   sqlcipher CLI 则引入运行时外部依赖，违反「仅标准库」红线
   （docs/06 §7）。
3. **红线与稳定性**：加密库读取不在本工具范围（docs/06 §6）；且该
   密钥在逆向侧此前的结论中属「硬编码占位」性质——占位密钥随版本
   更换会静默失效。
4. **Windows 侧不需要**：目录名即 QQ号（docs/08 §3.4）。

**何时重新评估**：若未来实现「消息记录解密按会话清理」（引用判定），
需解析 `nt_msg.db` 等消息库——那依赖运行时 enc_key 方案（OIDB 0x3294
下发），固定密钥只覆盖 login.db；届时一并评估 login.db 解析的收益。
