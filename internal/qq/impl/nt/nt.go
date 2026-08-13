// Package nt 是 NT 架构 QQ（nt_qq_<32hex> 布局）的逆向结论实现。
//
// 知识来源：macOS QQ 6.9.99 的逆向结论（docs/01-03、05）。
// NT 是跨平台架构，缓存布局（nt_qq_*/nt_data 下的 Pic/Video/...）
// 各平台共享；仅根路径随 OS 不同（见 roots_*.go）。
package nt

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"qqcleaner/internal/qq"
)

// NT 实现（无状态，方法即知识）。
type NT struct{}

var _ qq.Knowledge = &NT{}

func (*NT) Name() string      { return "nt" }
func (*NT) ScanCapable() bool { return true }

// ---- 实例目录 ----

var instanceRe = regexp.MustCompile(`^nt_qq_([0-9a-f]{32})$`)

func (*NT) InstanceDirs(root string) ([]qq.Instance, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []qq.Instance
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if m := instanceRe.FindStringSubmatch(e.Name()); m != nil {
			out = append(out, qq.Instance{DirName: e.Name(), Hash: m[1]})
		}
	}
	return out, nil
}

// ---- 账号识别三源（docs/02）----

func (k *NT) Identify(root string, inst qq.Instance) string {
	ntData := filepath.Join(root, inst.DirName, "nt_data")
	if qqNum, err := identifyFromMmkv(root, inst.Hash); err == nil && qqNum != "" {
		return qqNum
	}
	if qqNum, err := identifyFromUnitedConfig(ntData); err == nil && qqNum != "" {
		return qqNum
	}
	return ""
}

func identifyFromMmkv(qqRoot, instanceHash string) (string, error) {
	p := filepath.Join(qqRoot, "global", "nt_data", "mmkv", "mmkv.default")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`nt_qq_` + regexp.QuoteMeta(instanceHash) + `[^0-9]*?(\d{5,12})_flash`)
	if m := re.FindSubmatch(data); len(m) == 2 {
		return string(m[1]), nil
	}
	return "", nil
}

func identifyFromUnitedConfig(ntData string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(ntData, "UnitedConfig"))
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		n := e.Name()
		if n == "000" || len(n) < 5 || !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(n); err == nil {
			return n, nil
		}
	}
	return "", nil
}

// ---- 布局与命名（docs/01）----

var (
	bizDirs  = []string{"Pic", "Video", "Ptt", "File", "dataline", "Emoji"}
	skipDirs = map[string]bool{
		"mmkv": true, "msf": true, "OnlineStatus": true, "UnitedConfig": true,
		"config": true, "log": true, "log-cache": true, "avatar": true,
		"nt_db": true,
	}
	monthRe = regexp.MustCompile(`^\d{4}-\d{2}$`)
	nameRe  = regexp.MustCompile(`^([0-9a-f]{32})(?:_(\d+))?\.([A-Za-z0-9]+)$`)
)

func (*NT) BizDirs() []string         { return bizDirs }
func (*NT) SkipDirs() map[string]bool { return skipDirs }
func (*NT) IsMonthDir(name string) bool {
	return monthRe.MatchString(name)
}

func (*NT) ParseFilename(base string) (md5, sizeTag, ext string, ok bool) {
	m := nameRe.FindStringSubmatch(base)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], strings.ToLower(m[3]), true
}

func (*NT) Classify(segments []string) (biz, category, sub, month string) {
	if len(segments) < 2 {
		return "", "", "", ""
	}
	biz = strings.ToLower(segments[0])
	sub = segments[len(segments)-2]
	for _, s := range segments {
		if monthRe.MatchString(s) {
			month = s
			break
		}
	}
	switch biz {
	case "pic", "video", "ptt", "dataline":
		sub = lastOf(segments, "Ori", "Thumb", "OriTemp", "ThumbTemp")
		category = biz + "/" + strings.ToLower(sub)
	case "file":
		sub = lastOf(segments, "Ori", "Thumb", "OriTemp", "ThumbTemp", "file_assistant")
		category = biz + "/" + strings.ToLower(sub)
	case "emoji":
		sub, category = classifyEmoji(segments)
	default:
		category = "other"
	}
	return biz, category, sub, month
}

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
	case "BaseEmojiSyastems": // QQ 官方拼写如此（Syastems）
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

// ---- 白名单结构（docs/06 §2）----

func (*NT) Whitelisted(rel string, g qq.Gates) bool {
	segs := strings.Split(filepath.ToSlash(rel), "/")
	if len(segs) < 2 {
		return false
	}
	// 子类型 → 门控映射（docs/03 §6）。未知子类型 fail-closed。
	gateOf := func(sub string) (bool, bool) {
		switch sub {
		case "OriTemp", "ThumbTemp":
			return g.CleanTemp, true
		case "Ori":
			return g.CleanOri, true
		case "Thumb":
			return g.CleanThumb, true
		case "file_assistant":
			return g.CleanFile, true
		}
		return false, false
	}
	switch segs[0] {
	case "Pic", "Video", "Ptt", "dataline":
		// {biz}/{YYYY-MM}/{Ori|Thumb|OriTemp|ThumbTemp}/{file}
		if len(segs) < 4 || !monthRe.MatchString(segs[1]) {
			return false
		}
		on, ok := gateOf(segs[2])
		return ok && on
	case "File":
		// File/ 作为整体由 CleanFile 门控（docs/03 §3），不分子类型。
		if len(segs) < 3 {
			return false
		}
		_, ok := gateOf(segs[1])
		return ok && g.CleanFile
	case "Emoji":
		if len(segs) < 3 {
			return false
		}
		switch segs[1] {
		case "emoji-recv":
			// {Emoji}/emoji-recv/{YYYY-MM}/{Ori|Thumb|OriTemp|ThumbTemp}/{file}
			if len(segs) < 5 || !monthRe.MatchString(segs[2]) {
				return false
			}
			on, ok := gateOf(segs[3])
			return ok && on
		case "BaseEmojiSyastems":
			if len(segs) < 4 {
				return false
			}
			if segs[2] == "ThumbTemp" {
				return g.CleanTemp
			}
			return g.CleanBaseEmoji
		case "marketface":
			return g.CleanMarketface
		case "personal_emoji":
			return g.CleanPersonalEmoji
		}
		return false
	}
	return false
}

func (*NT) StateDirs() []string {
	return []string{"mmkv", "msf", "OnlineStatus", "UnitedConfig", "config", "log", "log-cache", "avatar", "nt_db"}
}

func (*NT) DBSuffixes() []string {
	return []string{".db", ".db-wal", ".db-shm", ".db-first.material", ".db-last.material"}
}
