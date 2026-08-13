// Package classify walks the whitelisted rich-media directories of one
// account's nt_data and classifies every file (biz / sub / category / month
// / md5 / size tag). It only reads; it never modifies QQ data.
//
// Reference: docs/01_data_structure.md §2-§4, docs/04_tool_design.md §4.2
package classify

import (
	"path/filepath"
	"regexp"
	"strings"
)

// FileEntry is one classified cache file. Category is the rules-level
// classification key; Sub is the leaf directory the file sits in (Ori/Thumb/
// OriTemp/ThumbTemp/...). MD5 and SizeTag come from the filename, so they
// are "" when the file does not follow the {md5}[_{size}].{ext} pattern.
type FileEntry struct {
	Path     string `json:"path"`
	Biz      string `json:"biz"`      // pic/video/ptt/emoji/file/dataline
	Sub      string `json:"sub"`      // leaf dir: Ori/Thumb/OriTemp/ThumbTemp/file_assistant/...
	Category string `json:"category"` // e.g. "pic/thumb", "emoji/emoji-recv/thumb"
	Month    string `json:"month"`    // YYYY-MM, "" when the path has none
	Size     int64  `json:"size"`
	MTime    int64  `json:"mtime"`
	MD5      string `json:"md5"`
	SizeTag  string `json:"sizeTag"` // the _{size} suffix ("0", "720", ...), "" if absent
	Ext      string `json:"ext"`     // extension without dot, lowercased
	IsThumb  bool   `json:"isThumb"`
	IsTemp   bool   `json:"isTemp"`
}

var (
	nameRe  = regexp.MustCompile(`^([0-9a-f]{32})(?:_(\d+))?\.([A-Za-z0-9]+)$`)
	monthRe = regexp.MustCompile(`^\d{4}-\d{2}$`)

	// BizDirs is the traversal whitelist (docs/01 §2.1): the rich-media cache
	// directories. Everything else under nt_data is configuration/state or
	// encrypted DB and is never scanned.
	BizDirs = []string{"Pic", "Video", "Ptt", "File", "dataline", "Emoji"}
)

// DefaultSkipDirs are state/config dirs that must never appear in a scan
// result (redline, docs/06 §2). They are filtered here AND re-verified in
// the clean layer.
var DefaultSkipDirs = map[string]bool{
	"mmkv": true, "msf": true, "OnlineStatus": true, "UnitedConfig": true,
	"config": true, "log": true, "log-cache": true, "avatar": true,
	"nt_db": true,
}

// classifyRelative parses a slash-separated path relative to nt_data into
// biz/category/sub/month. segments never contains the nt_data prefix.
func classifyRelative(segments []string) (biz, category, sub, month string) {
	if len(segments) < 2 {
		return "", "", "", ""
	}
	biz = strings.ToLower(segments[0])
	sub = segments[len(segments)-2] // leaf dir holding the file
	for _, s := range segments {
		if monthRe.MatchString(s) {
			month = s
			break
		}
	}
	switch biz {
	case "pic", "video", "ptt", "dataline":
		// {YYYY-MM}/{Ori|Thumb|OriTemp|ThumbTemp}/file
		sub = lastOf(segments, "Ori", "Thumb", "OriTemp", "ThumbTemp")
		category = biz + "/" + strings.ToLower(sub)
	case "file":
		// No month dir: {Ori|Thumb|ThumbTemp|file_assistant}/...
		sub = lastOf(segments, "Ori", "Thumb", "OriTemp", "ThumbTemp", "file_assistant")
		category = biz + "/" + strings.ToLower(sub)
	case "emoji":
		sub, category = classifyEmoji(segments)
	default:
		category = "other"
	}
	return biz, category, sub, month
}

// lastOf returns the first match scanning from the tail, so nested layouts
// (e.g. File/file_assistant/Thumb/...) resolve to the innermost sub dir.
func lastOf(segments []string, candidates ...string) string {
	for i := len(segments) - 1; i >= 0; i-- {
		for _, c := range candidates {
			if segments[i] == c {
				return c
			}
		}
	}
	return ""
}

// classifyEmoji maps Emoji's five subclasses (docs/01 §3) to
// (sub, category). Sub keeps the leaf dir for Ori/Thumb/Temp detection.
func classifyEmoji(segments []string) (sub, category string) {
	if len(segments) < 2 {
		return "", "emoji/other"
	}
	switch segments[1] {
	case "emoji-recv":
		// {YYYY-MM}/{Ori|Thumb}/file — same shape as Pic
		s := lastOf(segments, "Ori", "Thumb", "OriTemp", "ThumbTemp")
		if s == "" {
			s = segments[len(segments)-2]
		}
		return s, "emoji/emoji-recv/" + strings.ToLower(s)
	case "BaseEmojiSyastems": // note QQ's misspelling, docs/01 §3
		if len(segments) >= 3 && segments[2] == "ThumbTemp" {
			return "ThumbTemp", "emoji/base-emoji/thumbtemp"
		}
		return "EmojiSystermResource", "emoji/base-emoji/resource"
	case "marketface":
		return segments[len(segments)-2], "emoji/marketface"
	case "personal_emoji":
		s := lastOf(segments, "Ori", "Thumb")
		if s == "" {
			s = segments[len(segments)-2]
		}
		return s, "emoji/personal-emoji/" + strings.ToLower(s)
	case "emoji-related":
		return "emoji-related", "emoji/emoji-related"
	}
	return segments[len(segments)-2], "emoji/other"
}

// newEntry builds a FileEntry from an absolute path relative to ntData.
func newEntry(ntData, abs string, size int64, mtime int64) FileEntry {
	rel, err := filepath.Rel(ntData, abs)
	if err != nil {
		rel = abs
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	biz, category, sub, month := classifyRelative(segs)

	e := FileEntry{
		Path:     abs,
		Biz:      biz,
		Sub:      sub,
		Category: category,
		Month:    month,
		Size:     size,
		MTime:    mtime,
	}
	if m := nameRe.FindStringSubmatch(filepath.Base(abs)); m != nil {
		e.MD5, e.SizeTag, e.Ext = m[1], m[2], strings.ToLower(m[3])
	} else if i := strings.LastIndexByte(filepath.Base(abs), '.'); i > 0 {
		e.Ext = strings.ToLower(filepath.Base(abs)[i+1:])
	}
	subLower := strings.ToLower(e.Sub)
	e.IsThumb = subLower == "thumb"
	e.IsTemp = subLower == "oritemp" || subLower == "thumbtemp"
	return e
}
