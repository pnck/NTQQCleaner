package nt

import (
	"testing"

	"qqcleaner/internal/qq"
	"qqcleaner/internal/testutil"
)

// TestIdentifySources：三源账号识别（docs/02）。
func TestIdentifySources(t *testing.T) {
	f := testutil.BuildQQTree(t)
	k := &NT{}
	qa := k.Identify(f.Root, qqInstance(testutil.HashA))
	if qa != testutil.QQA {
		t.Fatalf("account A: got %q want %q", qa, testutil.QQA)
	}
	qb := k.Identify(f.Root, qqInstance(testutil.HashB))
	if qb != testutil.QQB {
		t.Fatalf("account B: got %q want %q", qb, testutil.QQB)
	}
	// Unknown hash → ""（UnitedConfig 兜底也 miss）
	if got := k.Identify(f.Root, qqInstance("deadbeefdeadbeefdeadbeefdeadbeef")); got != "" {
		t.Fatalf("unknown hash: got %q want empty", got)
	}
}

// TestParseFilename：{md5}[_{size}].{ext} 命名模板。
func TestParseFilename(t *testing.T) {
	k := &NT{}
	cases := []struct {
		name          string
		md5, tag, ext string
		ok            bool
	}{
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa09_720.png", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa09", "720", "png", true},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa10_0.jpg", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa10", "0", "jpg", true},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01.jpg", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01", "", "jpg", true},
		{"my.png", "", "", "", false},
	}
	for _, c := range cases {
		md5, tag, ext, ok := k.ParseFilename(c.name)
		if md5 != c.md5 || tag != c.tag || ext != c.ext || ok != c.ok {
			t.Errorf("ParseFilename(%q) = (%q,%q,%q,%v), want (%q,%q,%q,%v)",
				c.name, md5, tag, ext, ok, c.md5, c.tag, c.ext, c.ok)
		}
	}
}

// TestTypeScore：docs/03 §3 的目录优先级映射。
func TestTypeScore(t *testing.T) {
	k := &NT{}
	cases := map[string]int{
		"pic/oritemp":              0,
		"pic/thumb":                10,
		"emoji/emoji-recv/thumb":   10,
		"emoji/emoji-recv/ori":     20,
		"pic/ori":                  30,
		"video/ori":                30,
		"emoji/marketface":         35,
		"emoji/personal-emoji/ori": 40,
		"unknown/x":                40,
	}
	for cat, want := range cases {
		if got := k.TypeScore(cat); got != want {
			t.Errorf("TypeScore(%q) = %d, want %d", cat, got, want)
		}
	}
}

// TestClassifyEmoji：Emoji 五子类分类。
func TestClassifyEmoji(t *testing.T) {
	k := &NT{}
	cases := []struct {
		segs                      []string
		biz, category, sub, month string
	}{
		{[]string{"Emoji", "emoji-recv", "2024-05", "Thumb", "x_720.png"}, "emoji", "emoji/emoji-recv/thumb", "Thumb", "2024-05"},
		{[]string{"Emoji", "BaseEmojiSyastems", "ThumbTemp", "x.zip"}, "emoji", "emoji/base-emoji/thumbtemp", "ThumbTemp", ""},
		{[]string{"Emoji", "BaseEmojiSyastems", "EmojiSystermResource", "😀", "png", "x.png"}, "emoji", "emoji/base-emoji/resource", "EmojiSystermResource", ""},
		{[]string{"Emoji", "marketface", "123", "x.png"}, "emoji", "emoji/marketface", "123", ""},
		{[]string{"Emoji", "personal_emoji", "Ori", "my.png"}, "emoji", "emoji/personal-emoji/ori", "Ori", ""},
	}
	for _, c := range cases {
		biz, category, sub, month := k.Classify(c.segs)
		if biz != c.biz || category != c.category || sub != c.sub || month != c.month {
			t.Errorf("Classify(%v) = (%q,%q,%q,%q), want (%q,%q,%q,%q)",
				c.segs, biz, category, sub, month, c.biz, c.category, c.sub, c.month)
		}
	}
}

func qqInstance(hash string) qq.Instance {
	return qq.Instance{DirName: "nt_qq_" + hash, Hash: hash}
}
