package qq

import (
	"path/filepath"
	"strings"
)

// 白名单/黑名单的「结构知识」部分（docs/06 §2）。
// 政策（何时允许删）由 rules 层决定；结构事实（目录长什么样、
// 哪些目录是状态/加密库）由本层提供。

// Gates 是白名单结构校验需要的分类门控（由 rules.Config 映射而来）。
type Gates struct {
	CleanBaseEmoji     bool
	CleanMarketface    bool
	CleanPersonalEmoji bool
}

// Whitelisted 判断相对 nt_data 的路径是否属于「可能可删」的富媒体
// 缓存结构（深度+月份层校验）。黑名单与政策门控在 rules/clean 层。
func Whitelisted(rel string, g Gates) bool {
	segs := strings.Split(filepath.ToSlash(rel), "/")
	if len(segs) < 2 {
		return false
	}
	switch segs[0] {
	case "Pic", "Video", "Ptt", "dataline":
		// biz / {YYYY-MM} / {Ori|Thumb|Temp} / file
		return len(segs) >= 4 && MonthRe.MatchString(segs[1])
	case "File":
		// biz / {Ori|Thumb|Temp|file_assistant} / file（无月份层）
		return len(segs) >= 3
	case "Emoji":
		if len(segs) < 3 {
			return false
		}
		switch segs[1] {
		case "emoji-recv":
			// Emoji / emoji-recv / {YYYY-MM} / {Ori|Thumb} / file
			return len(segs) >= 4 && MonthRe.MatchString(segs[2])
		case "BaseEmojiSyastems":
			if len(segs) < 4 {
				return false
			}
			if segs[2] == "ThumbTemp" {
				return true // 下载残留，永远可清
			}
			return g.CleanBaseEmoji // EmojiSystermResource
		case "marketface":
			return g.CleanMarketface
		case "personal_emoji":
			return g.CleanPersonalEmoji
		}
		return false
	}
	return false
}

// StateDirs 是状态/配置目录名（永不删除）。
func StateDirs() []string {
	return []string{"mmkv", "msf", "OnlineStatus", "UnitedConfig", "config", "log", "log-cache", "avatar", "nt_db"}
}

// DBSuffixes 是 SQLite 及其附属文件后缀（永不删除）。
func DBSuffixes() []string {
	return []string{".db", ".db-wal", ".db-shm", ".db-first.material", ".db-last.material"}
}
