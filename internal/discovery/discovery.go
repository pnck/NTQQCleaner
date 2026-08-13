// Package discovery locates QQ data roots and the per-account instance
// directories (nt_qq_<32hex>) inside them.
//
// 逆向结论（目录命名、账号识别来源、各平台默认根路径）全部在
// internal/qq 知识层；本包只负责「找目录 + 组装结果」。
package discovery

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"qqcleaner/internal/qq"
)

// Account is one nt_qq_<32hex> instance inside a QQ data root.
type Account struct {
	Hash        string `json:"hash"`
	QQNum       string `json:"qqNum"` // plaintext QQ number; "" = unknown
	NtData      string `json:"ntData"`
	NtTemp      string `json:"ntTemp"`
	NtDb        string `json:"ntDb"` // never read, never cleaned
	LatestMonth string `json:"latestMonth"`
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
		if qq.InstanceRe.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Discover finds all account instances under root, identifies their QQ
// numbers, and sorts them newest-first by Pic activity (docs/02 §6).
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
			QQNum:       qq.IdentifyAccount(root, hash, ntData),
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
		if !e.IsDir() || !qq.MonthRe.MatchString(e.Name()) {
			continue
		}
		if e.Name() > latest {
			latest = e.Name()
		}
	}
	return latest
}
