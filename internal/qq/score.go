package qq

import "strings"

// 价值分级所需的逆向结论（docs/03）。

// QQOfficialThresholdSeconds = 259199 ≈ 3 天：QQ 官方清理的
// 时间基线（docs/03 §1）。
const QQOfficialThresholdSeconds int64 = 259199

// TypeScore 把分类键映射到目录类型优先级档（docs/03 §3）：
// P0 temp→0, P1 thumb→10, P2 emoji-recv ori→20, P3 ori→30,
// P4 marketface→35, P5 其余→40。
func TypeScore(category string) int {
	cat := strings.ToLower(category)
	switch {
	case strings.Contains(cat, "temp"):
		return 0
	case cat == "pic/thumb", cat == "video/thumb", cat == "ptt/thumb",
		cat == "file/thumb", cat == "dataline/thumb",
		cat == "emoji/emoji-recv/thumb":
		return 10
	case cat == "emoji/emoji-recv/ori":
		return 20
	case cat == "pic/ori", cat == "video/ori", cat == "dataline/ori", cat == "ptt/ori":
		return 30
	case cat == "emoji/marketface":
		return 35
	default:
		return 40
	}
}
