// Package legacy 是旧版 QQ（数字账号目录 + msg3.0.db）的占位实现。
//
// 旧版布局（docs/01 §1 提及的 <数字QQ号>/ 目录）尚未逆向出清理知识，
// 因此本实现 ScanCapable=false：只识别布局，扫描与清理一律拒绝
// （fail-closed）。逆向完成后补全本包即可，上层零改动。
package legacy

import (
	"os"
	"path/filepath"
	"strconv"

	"qqcleaner/internal/qq"
)

type Legacy struct{}

var _ qq.Knowledge = &Legacy{}

func (*Legacy) Name() string      { return "legacy" }
func (*Legacy) ScanCapable() bool { return false }

// InstanceDirs 识别数字账号目录（含 msg3.0.db 才算账号目录）。
func (*Legacy) InstanceDirs(root string) ([]qq.Instance, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []qq.Instance
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "msg3.0.db")); err == nil {
			out = append(out, qq.Instance{DirName: e.Name(), Hash: e.Name()})
		}
	}
	return out, nil
}

func (*Legacy) Identify(root string, inst qq.Instance) string { return "" }
func (*Legacy) BizDirs() []string                             { return nil }
func (*Legacy) SkipDirs() map[string]bool                     { return nil }
func (*Legacy) Classify(segments []string) (biz, category, sub, month string) {
	return "", "", "", ""
}
func (*Legacy) ParseFilename(base string) (md5, sizeTag, ext string, ok bool) {
	return "", "", "", false
}
func (*Legacy) IsMonthDir(name string) bool             { return false }
func (*Legacy) Whitelisted(rel string, g qq.Gates) bool { return false }
func (*Legacy) StateDirs() []string                     { return []string{"nt_db"} }
func (*Legacy) DBSuffixes() []string {
	return []string{".db", ".db-wal", ".db-shm"}
}
func (*Legacy) TypeScore(category string) int { return 40 }
