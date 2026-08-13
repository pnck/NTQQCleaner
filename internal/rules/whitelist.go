package rules

import (
	"path/filepath"
	"strings"

	"qqcleaner/internal/qq"
)

// 白名单/黑名单的政策层。结构事实（目录形态/状态目录/db 后缀）来自
// 注入的知识实现 k；本层把它与 Config 政策结合。

// Whitelisted 判断相对 nt_data 的路径是否可能被删除（docs/06 §2）。
// clean 层在删除前会再次调用本函数做最终校验。
func Whitelisted(k Knowledge, rel string, cfg Config) bool {
	return k.Whitelisted(rel, qq.Gates{
		CleanBaseEmoji:     cfg.CleanBaseEmoji,
		CleanMarketface:    cfg.CleanMarketface,
		CleanPersonalEmoji: cfg.CleanPersonalEmoji,
	})
}

// Blacklisted 判断绝对路径是否命中硬黑名单：SQLite 及附属文件、
// nt_db 树、状态/配置目录（docs/06 §2）。清单来自知识实现。
func Blacklisted(k Knowledge, abs string) bool {
	base := filepath.Base(abs)
	for _, suf := range k.DBSuffixes() {
		if strings.HasSuffix(strings.ToLower(base), suf) {
			return true
		}
	}
	dirs := make(map[string]bool, 8)
	for _, d := range k.StateDirs() {
		dirs[d] = true
	}
	segs := strings.Split(filepath.ToSlash(filepath.Clean(abs)), "/")
	for _, s := range segs {
		if dirs[s] {
			return true
		}
	}
	return false
}
