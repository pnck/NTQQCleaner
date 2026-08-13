// Package qq 是「逆向结论知识层」的唯一下沉点。
//
// 这里的一切都来自对 QQ 客户端存储/清理逻辑的逆向结论
// （docs/01-03、05）。不同操作系统上的 QQ 二进制可能被重新逆向分析、
// 结论可能不同 —— 届时只需重写本包（同一份上层逻辑不变）。
// 上层包（discovery/classify/rules）禁止内嵌逆向结论，一律引用本包。
package qq

import (
	"regexp"
	"strings"
)

// InstanceRe 匹配账号实例目录名：nt_qq_ + 32 位 hex。
// 目录名是 MD5(MD5(uid)+"nt_kernel")，不可反推 QQ 号（docs/01 §1）。
var InstanceRe = regexp.MustCompile(`^nt_qq_([0-9a-f]{32})$`)

// MonthRe 匹配 QQ 的 {YYYY-MM} 月份目录（docs/01 §2.2）。
var MonthRe = regexp.MustCompile(`^\d{4}-\d{2}$`)

// NameRe 匹配缓存文件命名模板 {32位hex md5}[_{size}].{ext}
// （docs/01 §2.2、§4.1；size 如 0/720/1080）。
var nameRe = regexp.MustCompile(`^([0-9a-f]{32})(?:_(\d+))?\.([A-Za-z0-9]+)$`)

// ParseFilename 按命名模板解析文件名；不匹配时 ok=false。
func ParseFilename(base string) (md5, sizeTag, ext string, ok bool) {
	m := nameRe.FindStringSubmatch(base)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], strings.ToLower(m[3]), true
}

// BizDirs 是遍历白名单：富媒体缓存目录（docs/01 §2.1、06 §2）。
var BizDirs = []string{"Pic", "Video", "Ptt", "File", "dataline", "Emoji"}

// DefaultSkipDirs 是永不扫描的状态/配置目录（docs/06 §2 黑名单的
// 目录部分；在 classify 层与 clean 层双重过滤）。
var DefaultSkipDirs = map[string]bool{
	"mmkv": true, "msf": true, "OnlineStatus": true, "UnitedConfig": true,
	"config": true, "log": true, "log-cache": true, "avatar": true,
	"nt_db": true,
}

// ClassifyRelative 把相对 nt_data 的路径段解析为
// (biz, category, sub, month)。category 是规则层使用的分类键；
// sub 是文件所在叶子目录名（Ori/Thumb/*Temp/file_assistant/...）。
func ClassifyRelative(segments []string) (biz, category, sub, month string) {
	if len(segments) < 2 {
		return "", "", "", ""
	}
	biz = strings.ToLower(segments[0])
	sub = segments[len(segments)-2] // 叶子目录
	for _, s := range segments {
		if MonthRe.MatchString(s) {
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
		// 无月份层：{Ori|Thumb|ThumbTemp|file_assistant}/...
		sub = lastOf(segments, "Ori", "Thumb", "OriTemp", "ThumbTemp", "file_assistant")
		category = biz + "/" + strings.ToLower(sub)
	case "emoji":
		sub, category = classifyEmoji(segments)
	default:
		category = "other"
	}
	return biz, category, sub, month
}

// lastOf 从尾部找第一个命中的候选目录名（嵌套布局取最内层）。
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

// classifyEmoji 映射 Emoji 五子类（docs/01 §3），注意 QQ 的拼写
// BaseEmojiSyastems（Syastems 为官方拼写错误，保留）。
func classifyEmoji(segments []string) (sub, category string) {
	if len(segments) < 2 {
		return "", "emoji/other"
	}
	switch segments[1] {
	case "emoji-recv":
		s := lastOf(segments, "Ori", "Thumb", "OriTemp", "ThumbTemp")
		if s == "" {
			s = segments[len(segments)-2]
		}
		return s, "emoji/emoji-recv/" + strings.ToLower(s)
	case "BaseEmojiSyastems":
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
