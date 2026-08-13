package rules

import "testing"

func TestWhitelisted(t *testing.T) {
	cfg := Default()
	cases := []struct {
		rel  string
		want bool
	}{
		{"Pic/2024-09/Thumb/abc_720.png", true},
		{"Pic/2024-09/Ori/abc.jpg", true},
		{"Pic/2024-09/OriTemp/abc.tmp", true},
		{"Video/2023-01/Thumb/abc_720.jpg", true},
		{"Ptt/2023-01/Ori/abc.amr", true},
		{"dataline/2023-01/Thumb/abc.png", true},
		{"Pic/Thumb/abc.png", false}, // missing month level
		{"Pic/abc.png", false},
		{"File/Thumb/abc.png", true},
		{"File/file_assistant/abc.txt", true},
		{"File/abc.txt", false},
		{"Emoji/emoji-recv/2024-05/Thumb/abc_720.png", true},
		{"Emoji/emoji-recv/Thumb/abc.png", false}, // missing month level
		{"Emoji/BaseEmojiSyastems/ThumbTemp/x.zip", true},
		{"Emoji/BaseEmojiSyastems/EmojiSystermResource/😀/png/x.png", false}, // needs CleanBaseEmoji
		{"Emoji/marketface/123/x.png", false},                               // needs CleanMarketface
		{"Emoji/personal_emoji/Ori/x.png", false},                           // needs CleanPersonalEmoji
		{"mmkv/mmkv.default", false},
		{"UnitedConfig/000/x", false},
		{"log/2026-08/x.log", false},
		{"", false},
		{"Pic", false},
	}
	for _, c := range cases {
		if got := Whitelisted(c.rel, cfg); got != c.want {
			t.Errorf("Whitelisted(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
	cfg.CleanBaseEmoji, cfg.CleanMarketface, cfg.CleanPersonalEmoji = true, true, true
	for _, rel := range []string{
		"Emoji/BaseEmojiSyastems/EmojiSystermResource/😀/png/x.png",
		"Emoji/marketface/123/x.png",
		"Emoji/personal_emoji/Ori/x.png",
	} {
		if !Whitelisted(rel, cfg) {
			t.Errorf("Whitelisted(%q) = false with gates on, want true", rel)
		}
	}
}

func TestBlacklisted(t *testing.T) {
	cases := []struct {
		abs  string
		want bool
	}{
		{"/data/nt_qq_xx/nt_db/message.db", true},
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Thumb/a.db-wal", true},
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Thumb/a.db-shm", true},
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Thumb/a.db-first.material", true},
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Thumb/a.db-last.material", true},
		{"/data/nt_qq_xx/nt_data/mmkv/mmkv.default", true},
		{"/data/nt_qq_xx/nt_data/msf/x", true},
		{"/data/nt_qq_xx/nt_data/OnlineStatus/x", true},
		{"/data/nt_qq_xx/nt_data/UnitedConfig/000/x", true},
		{"/data/nt_qq_xx/nt_data/log/2026-08/x.log", true},
		{"/data/nt_qq_xx/nt_data/avatar/x.jpg", true},
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Thumb/a.png", false},
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Ori/b.jpg", false},
	}
	for _, c := range cases {
		if got := Blacklisted(c.abs); got != c.want {
			t.Errorf("Blacklisted(%q) = %v, want %v", c.abs, got, c.want)
		}
	}
}
