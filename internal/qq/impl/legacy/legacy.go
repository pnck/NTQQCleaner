// Package legacy 是旧版 QQ（数字目录 + msg3.0.db）的兼容层实现
// （docs/08 §2.4、§3）。
//
// 逆向结论：Windows 上旧版账号目录顶层残留 25 个旧库（Msg3.0.db 等，
// 迁移 NT 后停更）与缓存目录（Image/Audio 的 $ 开头编码名、Video 的
// md5.png 平铺、无年月目录——与 NT 命名体系完全不同），QQ 官方清理器
// 只统计旧库不清理（ScanQQDirSize 实测结论；官方是否清 Image/Video
// 顶层缓存未确认 → 保守默认只统计）。
//
// 本实现对齐该行为：识别布局、识别账号（目录名即 QQ 号）、报告占用
// （Residues 只 stat 不读内容），但 ScanCapable=false——扫描/清理一律
// fail-closed。旧版若要做扫描需要一套独立知识，v1 不做。
package legacy

import (
	"os"
	"path/filepath"
	"regexp"

	"qqcleaner/internal/qq"
)

// Legacy 实现（无状态，方法即知识）。
type Legacy struct{}

var _ qq.Knowledge = &Legacy{}
var _ qq.ResidueReporter = &Legacy{}

func (*Legacy) Name() string      { return "legacy" }
func (*Legacy) ScanCapable() bool { return false }

var numDirRe = regexp.MustCompile(`^\d{5,12}$`)

// isLegacyDir：数字目录且不含 nt_qq/nt_data。含 nt_qq/nt_data 的数字
// 目录是 Windows NT 实例，由 nt 实现认领——两者互斥互补（docs/08 §3.3）。
func isLegacyDir(root, name string, isDir bool) bool {
	if !isDir || !numDirRe.MatchString(name) {
		return false
	}
	st, err := os.Stat(filepath.Join(root, name, "nt_qq", "nt_data"))
	return err != nil || !st.IsDir()
}

// ---- 实例目录与识别 ----

func (*Legacy) InstanceDirs(root string) ([]qq.Instance, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []qq.Instance
	for _, e := range entries {
		if isLegacyDir(root, e.Name(), e.IsDir()) {
			out = append(out, qq.Instance{DirName: e.Name(), Hash: e.Name()})
		}
	}
	return out, nil
}

// Identify：旧版目录名即 QQ 号（明文）。
func (*Legacy) Identify(root string, inst qq.Instance) string { return inst.DirName }

// Residues 报告旧版数据占用：实例目录顶层条目全量统计（整个目录就是
// 旧版数据，docs/08 §3.5）。只读、只 stat、不跟随符号链接。
func (*Legacy) Residues(root string, inst qq.Instance) ([]qq.LegacyResidue, error) {
	instDir := filepath.Join(root, inst.DirName)
	entries, err := os.ReadDir(instDir)
	if err != nil {
		return nil, err
	}
	var out []qq.LegacyResidue
	for _, e := range entries {
		p := filepath.Join(instDir, e.Name())
		size, count, err := qq.StatTree(p)
		if err != nil {
			continue // 不可读的条目跳过：报告宁缺毋假
		}
		out = append(out, qq.LegacyResidue{Path: p, Size: size, Count: count})
	}
	return out, nil
}

// ---- 布局与命名：fail-closed（不扫描，以下方法永不参与扫描管线）----

func (*Legacy) BizDirs() []string         { return nil }
func (*Legacy) SkipDirs() map[string]bool { return nil }
func (*Legacy) Classify(segments []string) (biz, category, sub, month string) {
	return "", "", "", ""
}
func (*Legacy) ParseFilename(base string) (md5, sizeTag, ext string, ok bool) {
	return "", "", "", false
}
func (*Legacy) IsMonthDir(name string) bool             { return false }
func (*Legacy) Whitelisted(rel string, g qq.Gates) bool { return false }

// ---- 黑名单底线（与版本无关，同 generic，docs/08 §4.1）----

func (*Legacy) StateDirs() []string { return []string{"nt_db"} }
func (*Legacy) DBSuffixes() []string {
	return []string{".db", ".db-wal", ".db-shm", ".db-first.material", ".db-last.material"}
}

// Probe 探测旧版布局：根下存在 ≥1 个旧版实例目录（docs/08 §3.3）。
func Probe(root string) qq.Knowledge {
	k := &Legacy{}
	insts, err := k.InstanceDirs(root)
	if err != nil || len(insts) == 0 {
		return nil
	}
	return k
}

func init() { qq.RegisterProbe(Probe) }
