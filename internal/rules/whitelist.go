package rules

import (
	"path/filepath"
	"strings"

	"qqcleaner/internal/qq"
)

// 白名单/黑名单的政策层。结构事实（目录形态/状态目录/db 后缀）在
// internal/qq 知识层；本层把它与 Config 政策结合。

// Whitelisted 判断相对 nt_data 的路径是否可能被删除（docs/06 §2）。
// clean 层在删除前会再次调用本函数做最终校验。
func Whitelisted(rel string, cfg Config) bool {
	return qq.Whitelisted(rel, qq.Gates{
		CleanBaseEmoji:     cfg.CleanBaseEmoji,
		CleanMarketface:    cfg.CleanMarketface,
		CleanPersonalEmoji: cfg.CleanPersonalEmoji,
	})
}

// hardBlockedDirs / hardBlockedSuffixes 由 qq 知识层提供，
// 在此构建快速查找表。
var (
	hardBlockedDirs     = toSet(qq.StateDirs())
	hardBlockedSuffixes = qq.DBSuffixes()
)

// Blacklisted 判断绝对路径是否命中硬黑名单：SQLite 及附属文件、
// nt_db 树、状态/配置目录（docs/06 §2）。
func Blacklisted(abs string) bool {
	base := filepath.Base(abs)
	for _, suf := range hardBlockedSuffixes {
		if strings.HasSuffix(strings.ToLower(base), suf) {
			return true
		}
	}
	segs := strings.Split(filepath.ToSlash(filepath.Clean(abs)), "/")
	for _, s := range segs {
		if hardBlockedDirs[s] {
			return true
		}
	}
	return false
}

func toSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
