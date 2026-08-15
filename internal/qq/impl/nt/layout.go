package nt

import (
	"os"
	"path/filepath"
	"regexp"
)

// layoutSpec 是 NT 布局在具体 OS 上的差异点（docs/08 §3.2）。逻辑本体
// （探测/实例枚举/账号识别/分类/白名单）跨平台共享，仅这几项随 OS
// 不同。做成数据而非代码分支的收益：linux 容器上的测试可以直接换
// currentSpec 验证 Windows 布局行为。
type layoutSpec struct {
	// instance 判断根目录条目是否为 NT 账号实例目录，返回该实例的
	// hash 键（mac: 目录名中的 32hex；windows: 目录名即 QQ 号）。
	instance func(root, name string, isDir bool) (hash string, ok bool)
	// ntRel 从实例目录到 nt_* 子目录组的相对路径（mac: ""；windows: "nt_qq"）。
	ntRel string
	// globalRel 从数据根到 global 目录的相对路径（mac: "global"；windows: "nt_qq/global"）。
	globalRel string
	// dirIsQQNum：实例目录名本身就是 QQ 号（windows 明文目录），
	// Identify 直接返回目录名，无需 mmkv/UnitedConfig 三源。
	dirIsQQNum bool
	// hasLegacyResidue：实例目录顶层除了 nt_* 子目录组之外还有旧版
	// 数据残留（windows：Msg3.0.db 等旧库；mac：无）。Residues 据此
	// 决定是否统计——显式字段而非「ntRel == ""」的隐式推断。
	hasLegacyResidue bool
}

var (
	macInstanceRe = regexp.MustCompile(`^nt_qq_([0-9a-f]{32})$`)
	winNumDirRe   = regexp.MustCompile(`^\d{5,12}$`)
)

// macInstance：nt_qq_<32hex> 目录（docs/01 §1）。
func macInstance(root, name string, isDir bool) (string, bool) {
	if !isDir {
		return "", false
	}
	m := macInstanceRe.FindStringSubmatch(name)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// winInstance：数字目录且含 nt_qq/nt_data 才是 NT 实例（docs/08 §3.3）。
// 第二个条件是关键判别：Windows 根下数字目录可能只是旧版残留
// （Msg3.0.db 等，无 nt_qq/），那类目录由 legacy probe 认领。
func winInstance(root, name string, isDir bool) (string, bool) {
	if !isDir || !winNumDirRe.MatchString(name) {
		return "", false
	}
	st, err := os.Stat(filepath.Join(root, name, "nt_qq", "nt_data"))
	if err != nil || !st.IsDir() {
		return "", false
	}
	return name, true
}

var (
	darwinSpec  = layoutSpec{instance: macInstance, ntRel: "", globalRel: "global", dirIsQQNum: false, hasLegacyResidue: false}
	windowsSpec = layoutSpec{instance: winInstance, ntRel: "nt_qq", globalRel: filepath.Join("nt_qq", "global"), dirIsQQNum: true, hasLegacyResidue: true}
	// linuxSpec：Linux 无 NT QQ 默认根，用户指定 --root 时按 mac 风格
	// 枚举（nt_qq_<32hex>）。
	linuxSpec = darwinSpec

	// currentSpec 由 spec_<os>.go（build-tagged）的 init 覆盖为当前
	// OS 的 spec；测试可直接替换（docs/08 §5）。
	currentSpec = darwinSpec
)
