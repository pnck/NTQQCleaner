package rules

import (
	"strings"

	"qqcleaner/internal/classify"
	"qqcleaner/internal/qq"
)

// Knowledge 是 rules 层对 qq 知识实现的窄接口（消费方定义）：
// reason 标签与白名单/黑名单判定只需要结构事实。
type Knowledge interface {
	Whitelisted(rel string, gates qq.Gates) bool
	StateDirs() []string
	DBSuffixes() []string
}

// MD5Index indexes entries by filename md5 (docs/04 §4.3): for every md5,
// whether an Ori original exists and how many times the md5 appears.
type MD5Index map[string]struct {
	HasOri bool
	Count  int
}

// BuildMD5Index indexes entries by filename md5.
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

// Reason 返回短标签（如「缩略图」「重复出现」）；UI 悬浮 tooltip 用
// 标签词典解释含义，CLI manifest 直接展示标签。
func Reason(e classify.FileEntry, idx MD5Index) string {
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
