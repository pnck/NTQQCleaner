// Package discovery locates QQ data roots and the per-account instance
// directories inside them. 布局知识全部来自 qq 层（Detect 分派），
// 本包只负责「找目录 + 组装结果」。
package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"qqcleaner/internal/qq"
)

// Account is one account instance inside a QQ data root.
type Account struct {
	Hash        string `json:"hash"`
	QQNum       string `json:"qqNum"` // plaintext QQ number; "" = unknown
	NtData      string `json:"ntData"`
	NtTemp      string `json:"ntTemp"`
	NtDb        string `json:"ntDb"` // never read, never cleaned
	LatestMonth string `json:"latestMonth"`
}

// IsInstanceRoot reports whether root looks like a QQ data root
// (detected layout has at least one account instance).
func IsInstanceRoot(root string) bool {
	k := qq.Detect(root)
	insts, err := k.InstanceDirs(root)
	return err == nil && len(insts) > 0
}

// Discover detects the QQ layout, finds all account instances, identifies
// their QQ numbers, and sorts newest-first by Pic activity (docs/02 §6).
// Unknown accounts sort last. Never writes to the QQ data root.
// 不支持的布局（generic 兜底）返回错误，fail-closed。
func Discover(root string) ([]Account, error) {
	k := qq.Detect(root)
	if !k.ScanCapable() {
		return nil, fmt.Errorf("unsupported QQ data layout (detected: %s)", k.Name())
	}
	insts, err := k.InstanceDirs(root)
	if err != nil {
		return nil, err
	}
	accounts := make([]Account, 0, len(insts))
	for _, inst := range insts {
		ntData := filepath.Join(root, inst.DirName, "nt_data")
		accounts = append(accounts, Account{
			Hash:        inst.Hash,
			QQNum:       k.Identify(root, inst),
			NtData:      ntData,
			NtTemp:      filepath.Join(root, inst.DirName, "nt_temp"),
			NtDb:        filepath.Join(root, inst.DirName, "nt_db"),
			LatestMonth: latestPicMonth(k, ntData),
		})
	}
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

// latestPicMonth returns the newest {YYYY-MM} month dir under nt_data/Pic/
// (docs/02 §6 新旧账号排序依据)。
func latestPicMonth(k qq.Knowledge, ntData string) string {
	entries, err := os.ReadDir(filepath.Join(ntData, "Pic"))
	if err != nil {
		return ""
	}
	latest := ""
	for _, e := range entries {
		if !e.IsDir() || !k.IsMonthDir(e.Name()) {
			continue
		}
		if e.Name() > latest {
			latest = e.Name()
		}
	}
	return latest
}
