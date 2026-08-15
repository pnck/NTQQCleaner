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

// TestClassifyFlashfransfer：闪传中转区（biz 29，docs/01 §2.3、03 §1）
// 三个子目录的分类——官方「缓存文件」category 成员。
func TestClassifyFlashfransfer(t *testing.T) {
	k := &NT{}
	cases := []struct {
		segs               []string
		biz, category, sub string
	}{
		{[]string{"flashfransfer", "upload_temp", "x.part"}, "flashfransfer", "flashfransfer/upload_temp", "upload_temp"},
		{[]string{"flashfransfer", "download_temp", "y"}, "flashfransfer", "flashfransfer/download_temp", "download_temp"},
		{[]string{"flashfransfer", "thumb", "z.png"}, "flashfransfer", "flashfransfer/thumb", "thumb"},
	}
	for _, c := range cases {
		biz, category, sub, month := k.Classify(c.segs)
		if biz != c.biz || category != c.category || sub != c.sub || month != "" {
			t.Errorf("Classify(%v) = (%q,%q,%q,%q), want (%q,%q,%q,\"\")",
				c.segs, biz, category, sub, month, c.biz, c.category, c.sub)
		}
	}
}
