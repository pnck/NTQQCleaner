//go:build linux

package qq

// RootCandidates Linux：无稳定的默认路径（各发行版/安装方式差异大），
// 由用户 --root / UI 指定。
func RootCandidates() []string { return nil }
