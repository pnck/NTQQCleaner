// Package discovery locates QQ data roots and the per-account instance
// directories (nt_qq_<32hex>) inside them, resolving each instance hash
// to a plaintext QQ number where possible.
//
// Reference: docs/01_data_structure.md, docs/02_account_identification.md
package discovery

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// instanceRe matches an account instance directory name: nt_qq_ + 32 hex chars.
// The hash is MD5(MD5(uid)+"nt_kernel") and must NOT be reversed into a QQ
// number — use identify.go instead.
var instanceRe = regexp.MustCompile(`^nt_qq_([0-9a-f]{32})$`)

// Account is one nt_qq_<32hex> instance inside a QQ data root.
type Account struct {
	Hash        string `json:"hash"`
	QQNum       string `json:"qqNum"` // plaintext QQ number; "" = unknown
	NtData      string `json:"ntData"`
	NtTemp      string `json:"ntTemp"`
	NtDb        string `json:"ntDb"` // never read, never cleaned
	LatestMonth string `json:"latestMonth"`
}

// RootCandidates returns platform-default QQ data root paths, most likely first.
// Users can always override with --root / the UI picker.
func RootCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []string
	switch {
	case isDarwin():
		out = append(out, filepath.Join(home,
			"Library", "Containers", "com.tencent.qq", "Data", "Library", "Application Support", "QQ"))
	case isWindows():
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			out = append(out, filepath.Join(appdata, "Tencent", "QQ"))
		}
		if profile := os.Getenv("USERPROFILE"); profile != "" {
			out = append(out, filepath.Join(profile, "Documents", "Tencent Files"))
		}
	}
	return out
}

// IsInstanceRoot reports whether root looks like a QQ data root
// (contains at least one nt_qq_* directory).
func IsInstanceRoot(root string) bool {
	accs, _ := instanceDirs(root)
	return len(accs) > 0
}

// instanceDirs lists nt_qq_<hash> directory names under root.
func instanceDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if instanceRe.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Discover finds all account instances under root, identifies their QQ
// numbers, and sorts them newest-first by Pic activity (see docs/02 §6).
// Unknown accounts sort last. Never writes to the QQ data root.
func Discover(root string) ([]Account, error) {
	names, err := instanceDirs(root)
	if err != nil {
		return nil, err
	}
	accounts := make([]Account, 0, len(names))
	for _, name := range names {
		hash := strings.TrimPrefix(name, "nt_qq_")
		ntData := filepath.Join(root, name, "nt_data")
		accounts = append(accounts, Account{
			Hash:        hash,
			QQNum:       IdentifyAccount(root, hash, ntData),
			NtData:      ntData,
			NtTemp:      filepath.Join(root, name, "nt_temp"),
			NtDb:        filepath.Join(root, name, "nt_db"),
			LatestMonth: latestPicMonth(ntData),
		})
	}
	// Newest activity first; unknown months ("") sort last.
	sort.Slice(accounts, func(i, j int) bool {
		a, b := accounts[i].LatestMonth, accounts[j].LatestMonth
		if a == b {
			return accounts[i].QQNum != "" // prefer identified on ties
		}
		if a == "" {
			return false
		}
		if b == "" {
			return true
		}
		return a > b
	})
	return accounts, nil
}

// latestPicMonth returns the newest {YYYY-MM} month dir under nt_data/Pic/,
// or "" if Pic does not exist or has no month dirs. Used to order accounts
// by recency (docs/02 §6).
func latestPicMonth(ntData string) string {
	entries, err := os.ReadDir(filepath.Join(ntData, "Pic"))
	if err != nil {
		return ""
	}
	latest := ""
	for _, e := range entries {
		if !e.IsDir() || !monthRe.MatchString(e.Name()) {
			continue
		}
		if e.Name() > latest {
			latest = e.Name()
		}
	}
	return latest
}
