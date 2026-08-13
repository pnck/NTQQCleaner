package rules

import (
	"strings"
	"time"

	"qqcleaner/internal/classify"
)

// Tier labels (docs/03 §4).
const (
	TierSafe    = "safe"
	TierSuggest = "suggest"
	TierCaution = "caution"
	TierKeep    = "keep"
)

// MD5Index supports redundancy scoring: for every md5, whether an Ori
// original exists and how many times the md5 appears.
type MD5Index map[string]struct {
	HasOri bool
	Count  int
}

// BuildMD5Index indexes entries by filename md5 (docs/04 §4.3).
func BuildMD5Index(entries []classify.FileEntry) MD5Index {
	idx := make(MD5Index, len(entries)/2+1)
	for _, e := range entries {
		if e.MD5 == "" {
			continue
		}
		v := idx[e.MD5]
		v.Count++
		if strings.EqualFold(e.Sub, "Ori") {
			v.HasOri = true
		}
		idx[e.MD5] = v
	}
	return idx
}

// typeScore maps a category to its priority bucket (docs/03 §3).
// P0 temp → 0, P1 thumb → 10, P2 emoji-recv ori → 20, P3 ori → 30,
// P4 marketface → 35, P5 everything else (File/, personal emoji, base
// emoji resources, ...) → 40.
func typeScore(category string) int {
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

// timeScore maps age to points (docs/03 §4). L0 (just past QQ's 3-day
// threshold) contributes nothing; the monthly-archive rule (L5) dominates
// months older than the archive cutoff. Files at or below the threshold
// return 0 — Tier applies the freshness keep-gate separately.
func timeScore(e classify.FileEntry, cfg Config, now time.Time) int {
	age := now.Unix() - e.MTime
	if age <= cfg.DefaultThresholdSeconds {
		return 0
	}
	score := 0
	for i, days := range cfg.TimeTierDays {
		if age > days*86400 {
			switch i {
			case 0:
				score = 5 // L1
			case 1:
				score = 10 // L2
			case 2:
				score = 18 // L3
			case 3:
				score = 25 // L4
			}
		}
	}
	// L5: whole month older than the archive cutoff (docs/03 §2).
	if e.Month != "" {
		if m, err := time.Parse("2006-01", e.Month); err == nil {
			monthsOld := (now.Year()-m.Year())*12 + int(now.Month()-m.Month())
			if monthsOld > cfg.ArchiveMonthOlderThanMonths {
				score = 30
			}
		}
	}
	return score
}

// redundancyScore rewards files whose content is redundant (docs/03 §4):
// a thumbnail whose Ori original still exists (+20), or any md5 appearing
// more than once (+10).
func redundancyScore(e classify.FileEntry, idx MD5Index) int {
	if e.MD5 == "" {
		return 0
	}
	v, ok := idx[e.MD5]
	if !ok {
		return 0
	}
	if v.HasOri && !strings.EqualFold(e.Sub, "Ori") {
		return 20
	}
	if v.Count > 1 {
		return 10
	}
	return 0
}

func sizeScore(size int64) int {
	switch {
	case size > 10<<20:
		return 10
	case size > 1<<20:
		return 7
	default:
		return 3
	}
}

// Score computes value_score 0..100 (lower = safer to clean).
// score = type + time + redundancy + size (docs/03 §4).
func Score(e classify.FileEntry, idx MD5Index, cfg Config, now time.Time) int {
	return typeScore(e.Category) + timeScore(e, cfg, now) + redundancyScore(e, idx) + sizeScore(e.Size)
}

// fresh reports whether the file is at or below QQ's 3-day threshold
// (docs/03 §1: only mtime older than 259199s may ever be considered).
func fresh(e classify.FileEntry, cfg Config, now time.Time) bool {
	return now.Unix()-e.MTime <= cfg.DefaultThresholdSeconds
}

// Tier maps an entry to a tier label, applying the redlines and category
// gates (docs/03 §4, §7): *Temp is always safe; fresh files are always
// keep; disabled categories are report-only; 🟠 caution requires the
// aggressive flag.
func Tier(e classify.FileEntry, score int, cfg Config, now time.Time) string {
	if e.IsTemp {
		if cfg.CleanTemp {
			return TierSafe
		}
		return TierKeep
	}
	if fresh(e, cfg, now) {
		return TierKeep // too new to touch
	}
	cat := strings.ToLower(e.Category)
	// Category gates — order matters (specific first). File/ is gated as a
	// whole by CleanFile (docs/03 §3 P5 treats it as one cautious unit),
	// including its Thumb subtree.
	switch {
	case strings.HasPrefix(cat, "emoji/personal-emoji"):
		if !cfg.CleanPersonalEmoji {
			return TierKeep
		}
	case strings.HasPrefix(cat, "file/"):
		if !cfg.CleanFile {
			return TierKeep
		}
	case strings.HasSuffix(cat, "/thumb"):
		if !cfg.CleanThumb {
			return TierKeep
		}
	case strings.HasSuffix(cat, "/ori"):
		if !cfg.CleanOri {
			return TierKeep
		}
	case cat == "emoji/marketface":
		if !cfg.CleanMarketface {
			return TierKeep
		}
	case cat == "emoji/base-emoji/resource":
		if !cfg.CleanBaseEmoji {
			return TierKeep
		}
	case strings.HasPrefix(cat, "emoji/emoji-related"), cat == "emoji/other", cat == "other":
		return TierKeep
	}
	switch {
	case score <= cfg.ScoreThresholds.Safe:
		return TierSafe
	case score <= cfg.ScoreThresholds.Suggest:
		return TierSuggest
	case score <= cfg.ScoreThresholds.Caution:
		if cfg.Aggressive {
			return TierCaution
		}
		return TierKeep
	default:
		return TierKeep
	}
}

// Reason 返回短标签（如「缩略图」「重复出现」）；UI 悬浮 tooltip 用
// 标签词典解释含义，CLI manifest 直接展示标签。
func Reason(e classify.FileEntry, tier string, idx MD5Index) string {
	_ = tier // 保留参数位（CLI manifest 分级仍由 Tier 单独表达）
	var parts []string
	switch {
	case e.IsTemp:
		parts = append(parts, "下载中断残留")
	case e.IsThumb:
		parts = append(parts, "缩略图")
	case strings.Contains(strings.ToLower(e.Category), "marketface"):
		parts = append(parts, "表情包")
	case strings.Contains(strings.ToLower(e.Category), "personal-emoji"):
		parts = append(parts, "个人表情")
	case strings.HasSuffix(strings.ToLower(e.Category), "/ori"):
		parts = append(parts, "原图/原文件")
	}
	if v, ok := idx[e.MD5]; ok && e.MD5 != "" {
		if v.HasOri && !strings.EqualFold(e.Sub, "Ori") {
			parts = append(parts, "原图仍在")
		} else if v.Count > 1 {
			parts = append(parts, "重复出现")
		}
	}
	if len(parts) == 0 {
		return "缓存文件"
	}
	return strings.Join(parts, "；")
}
