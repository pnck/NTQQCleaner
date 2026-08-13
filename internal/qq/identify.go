package qq

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// 账号识别：三个完全明文的来源把实例 hash（nt_qq_<32hex>）映射到
// QQ 号。均不触碰加密的 nt_db（docs/02）。

// IdentifyFromMmkv 来源一（最可靠）：global/nt_data/mmkv/mmkv.default
// 含明文 key：.../nt_qq_<hash>/nt_data/flashfransfer+<qq>_flash_...。
func IdentifyFromMmkv(qqRoot, instanceHash string) (string, error) {
	p := filepath.Join(qqRoot, "global", "nt_data", "mmkv", "mmkv.default")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`nt_qq_` + regexp.QuoteMeta(instanceHash) + `[^0-9]*?(\d{5,12})_flash`)
	if m := re.FindSubmatch(data); len(m) == 2 {
		return string(m[1]), nil
	}
	return "", nil
}

// IdentifyFromUnitedConfig 来源二：nt_data/UnitedConfig/ 除 "000" 外的
// 数字子目录名即 QQ 号。
func IdentifyFromUnitedConfig(ntData string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(ntData, "UnitedConfig"))
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		n := e.Name()
		if n == "000" || len(n) < 5 || !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(n); err == nil {
			return n, nil
		}
	}
	return "", nil
}

// ListLoggedAccounts 来源三（交叉验证）：global/nt_data/Login/ 下
// 每个登录过的账号有一个 ".<qq号>" 空文件。只列出账号，不关联目录。
func ListLoggedAccounts(qqRoot string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(qqRoot, "global", "nt_data", "Login"))
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

// IdentifyAccount 三源综合（docs/02 §5）：mmkv 最可靠，
// UnitedConfig 兜底，识别不到返回 ""（由上层显示 unknown）。
func IdentifyAccount(qqRoot, instanceHash, ntData string) string {
	qq, err := IdentifyFromMmkv(qqRoot, instanceHash)
	if err == nil && qq != "" {
		return qq
	}
	qq, err = IdentifyFromUnitedConfig(ntData)
	if err == nil && qq != "" {
		return qq
	}
	return ""
}
