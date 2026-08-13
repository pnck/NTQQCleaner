package qq

import (
	"testing"

	"qqcleaner/internal/testutil"
)

func TestIdentifyFromMmkv(t *testing.T) {
	f := testutil.BuildQQTree(t)
	qa, err := IdentifyFromMmkv(f.Root, testutil.HashA)
	if err != nil {
		t.Fatal(err)
	}
	if qa != testutil.QQA {
		t.Fatalf("account A: got %q want %q", qa, testutil.QQA)
	}
	qb, err := IdentifyFromMmkv(f.Root, testutil.HashB)
	if err != nil {
		t.Fatal(err)
	}
	if qb != testutil.QQB {
		t.Fatalf("account B: got %q want %q", qb, testutil.QQB)
	}
	// Unknown hash: no match, no error.
	qqNum, err := IdentifyFromMmkv(f.Root, "deadbeefdeadbeefdeadbeefdeadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if qqNum != "" {
		t.Fatalf("unknown hash: got %q want empty", qqNum)
	}
}

func TestIdentifyFromUnitedConfig(t *testing.T) {
	f := testutil.BuildQQTree(t)
	qqNum, err := IdentifyFromUnitedConfig(f.NtDataA)
	if err != nil {
		t.Fatal(err)
	}
	if qqNum != testutil.QQA {
		t.Fatalf("got %q want %q", qqNum, testutil.QQA)
	}
	// Account B only has "000" → no QQ number (mmkv is the fallback source).
	qqNum, err = IdentifyFromUnitedConfig(f.NtDataB)
	if err != nil {
		t.Fatal(err)
	}
	if qqNum != "" {
		t.Fatalf("got %q want empty", qqNum)
	}
}

func TestListLoggedAccounts(t *testing.T) {
	f := testutil.BuildQQTree(t)
	got, err := ListLoggedAccounts(f.Root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{testutil.QQA: true, testutil.QQB: true}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for _, qqNum := range got {
		if !want[qqNum] {
			t.Fatalf("unexpected account %q", qqNum)
		}
	}
}

func TestWhitelistedStructure(t *testing.T) {
	g := Gates{CleanBaseEmoji: true, CleanMarketface: true, CleanPersonalEmoji: true}
	cases := []struct {
		rel  string
		want bool
	}{
		{"Pic/2024-09/Thumb/abc_720.png", true},
		{"Pic/Thumb/abc.png", false}, // 缺月份层
		{"File/Thumb/abc.png", true},
		{"File/abc.txt", false},
		{"Emoji/emoji-recv/2024-05/Thumb/abc_720.png", true},
		{"Emoji/emoji-recv/Thumb/abc.png", false}, // 缺月份层
		{"Emoji/BaseEmojiSyastems/ThumbTemp/x.zip", true},
		{"Emoji/BaseEmojiSyastems/EmojiSystermResource/😀/png/x.png", true}, // 门控开
		{"mmkv/mmkv.default", false},
		{"", false},
	}
	for _, c := range cases {
		if got := Whitelisted(c.rel, g); got != c.want {
			t.Errorf("Whitelisted(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
	// 门控关：三个 Emoji 子类不再白名单
	g = Gates{}
	for _, rel := range []string{
		"Emoji/BaseEmojiSyastems/EmojiSystermResource/😀/png/x.png",
		"Emoji/marketface/123/x.png",
		"Emoji/personal_emoji/Ori/x.png",
	} {
		if Whitelisted(rel, g) {
			t.Errorf("Whitelisted(%q) = true with gates off, want false", rel)
		}
	}
}
