package discovery

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Account identification: three fully-plaintext sources map an instance hash
// (nt_qq_<32hex>) to a QQ number. None of them touch the encrypted nt_db.
// Reference: docs/02_account_identification.md

var monthRe = regexp.MustCompile(`^\d{4}-\d{2}$`)

// IdentifyFromMmkv is source #1 (most reliable): global/nt_data/mmkv/mmkv.default
// contains plaintext keys like ".../nt_qq_<hash>/nt_data/flashfransfer+<qq>_flash_...".
func IdentifyFromMmkv(qqRoot, instanceHash string) (string, error) {
	p := filepath.Join(qqRoot, "global", "nt_data", "mmkv", "mmkv.default")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	// nt_qq_<hash> then any non-digits (path separators, "flashfransfer+"),
	// then a 5-12 digit QQ number, then "_flash" (so the digits are a QQ
	// number, not some other number in the path).
	re := regexp.MustCompile(`nt_qq_` + regexp.QuoteMeta(instanceHash) + `[^0-9]*?(\d{5,12})_flash`)
	if m := re.FindSubmatch(data); len(m) == 2 {
		return string(m[1]), nil
	}
	return "", nil
}

// IdentifyFromUnitedConfig is source #2: nt_data/UnitedConfig/ contains a
// "000" default dir and, per account, a subdirectory named after the QQ number.
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

// ListLoggedAccounts is source #3 (cross-check only): global/nt_data/Login/
// holds a zero-byte ".<qq>" marker file per account that ever logged in.
// It lists accounts but does not associate them with instance dirs.
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

// IdentifyAccount combines the three sources: mmkv first, UnitedConfig
// fallback, "unknown" when nothing matches (docs/02 §5).
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
