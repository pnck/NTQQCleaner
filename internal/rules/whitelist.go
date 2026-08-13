package rules

import (
	"path/filepath"
	"regexp"
	"strings"
)

// monthRe matches QQ's {YYYY-MM} month directories.
var monthRe = regexp.MustCompile(`^\d{4}-\d{2}$`)

// Whitelisted reports whether a slash-separated path relative to an
// account's nt_data may ever be deleted by this tool (docs/06 §2):
//
//	Pic|Video|Ptt|File|dataline/**        (Ori/Thumb/Temp subtrees)
//	Emoji/emoji-recv/**                    (always)
//	Emoji/BaseEmojiSyastems/ThumbTemp/**   (always; download residue)
//	Emoji/marketface|personal_emoji|BaseEmojiSyastems/EmojiSystermResource
//	                                       (only when their cfg gate is on)
//
// The clean layer re-verifies every file against this function plus
// Blacklisted immediately before deletion.
func Whitelisted(rel string, cfg Config) bool {
	segs := strings.Split(filepath.ToSlash(rel), "/")
	if len(segs) < 2 {
		return false
	}
	switch segs[0] {
	case "Pic", "Video", "Ptt", "dataline":
		// biz / {YYYY-MM} / {Ori|Thumb|Temp} / file
		return len(segs) >= 4 && monthRe.MatchString(segs[1])
	case "File":
		// biz / {Ori|Thumb|Temp|file_assistant} / file (no month dir)
		return len(segs) >= 3
	case "Emoji":
		if len(segs) < 3 {
			return false
		}
		switch segs[1] {
		case "emoji-recv":
			// Emoji / emoji-recv / {YYYY-MM} / {Ori|Thumb} / file
			return len(segs) >= 4 && monthRe.MatchString(segs[2])
		case "BaseEmojiSyastems":
			if len(segs) < 4 {
				return false
			}
			if segs[2] == "ThumbTemp" {
				return true // download residue, always cleanable
			}
			return cfg.CleanBaseEmoji // EmojiSystermResource
		case "marketface":
			return cfg.CleanMarketface
		case "personal_emoji":
			return cfg.CleanPersonalEmoji
		}
		return false
	}
	return false
}

// hardBlockedSuffixes are SQLite artifacts and encrypted-DB sidecars that
// must never be deleted, no matter where they appear (docs/06 §2).
var hardBlockedSuffixes = []string{
	".db", ".db-wal", ".db-shm", ".db-first.material", ".db-last.material",
}

// hardBlockedDirs are state/config dirs that must never be deleted.
var hardBlockedDirs = map[string]bool{
	"mmkv": true, "msf": true, "OnlineStatus": true, "UnitedConfig": true,
	"config": true, "log": true, "log-cache": true, "avatar": true,
	"nt_db": true,
}

// Blacklisted reports hard-blocked absolute paths (docs/06 §2): SQLite
// artifacts, nt_db trees, and state/config dirs. It is applied at the
// clean layer on top of Whitelisted.
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
