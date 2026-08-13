// Package classify walks the whitelisted rich-media directories of one
// account's nt_data and classifies every file (biz / sub / category / month
// / md5 / size tag). It only reads; it never modifies QQ data.
//
// Reference: docs/01_data_structure.md §2-§4, docs/04_tool_design.md §4.2
package classify

import (
	"path/filepath"
	"strings"

	"qqcleaner/internal/qq"
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

// BizDirs 等遍历白名单与分类逻辑见 qq 包（逆向结论层）。
// 本包只负责「走文件系统 + 组装 FileEntry」，不含逆向知识。
var (
	BizDirs        = qq.BizDirs
	DefaultSkipDirs = qq.DefaultSkipDirs
)

// newEntry builds a FileEntry from an absolute path relative to ntData.
func newEntry(ntData, abs string, size int64, mtime int64) FileEntry {
	rel, err := filepath.Rel(ntData, abs)
	if err != nil {
		rel = abs
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	biz, category, sub, month := qq.ClassifyRelative(segs)

	e := FileEntry{
		Path:     abs,
		Biz:      biz,
		Sub:      sub,
		Category: category,
		Month:    month,
		Size:     size,
		MTime:    mtime,
	}
	if md5, tag, ext, ok := qq.ParseFilename(filepath.Base(abs)); ok {
		e.MD5, e.SizeTag, e.Ext = md5, tag, ext
	} else if i := strings.LastIndexByte(filepath.Base(abs), '.'); i > 0 {
		e.Ext = strings.ToLower(filepath.Base(abs)[i+1:])
	}
	subLower := strings.ToLower(e.Sub)
	e.IsThumb = subLower == "thumb"
	e.IsTemp = subLower == "oritemp" || subLower == "thumbtemp"
	return e
}
