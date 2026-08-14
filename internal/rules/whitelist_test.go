package rules

import (
	"testing"
)

func TestWhitelisted(t *testing.T) {
	// 结构用例：门控全部打开时，纯结构校验（黑名单类别另测）。
	open := AllGatesConfig()

	structural := []struct {
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
		{"Emoji/BaseEmojiSyastems/EmojiSystermResource/😀/png/x.png", true},
		{"Emoji/marketface/123/x.png", true},
		{"Emoji/personal_emoji/Ori/x.png", true},
		{"dataline/.tmp/output.mp4.x.NFC", true}, // 传输残留（clean_temp）
		{"dataline/.tmp", false},
		{"avatar/x.jpg", true}, // 头像缓存（高级 opt-in）
		{"log/2026-08/x.log", true},  // 运行日志（clean_log）
		{"log-cache/x", true},
		{"mmkv/mmkv.default", false},
		{"UnitedConfig/000/x", false},
		{"", false},
		{"Pic", false},
		{"Pic/2024-09/Xyz/abc.png", false}, // unknown sub dir fails closed
	}
	for _, c := range structural {
		if got := Whitelisted(ntK(), c.rel, open); got != c.want {
			t.Errorf("Whitelisted(%q) with gates open = %v, want %v", c.rel, got, c.want)
		}
	}

	// 门控用例：CLI 保守默认（clean_ori/clean_file/emoji 全部关闭）。
	def := Default()
	gated := []struct {
		rel  string
		want bool
	}{
		{"Pic/2024-09/Thumb/abc_720.png", true},     // clean_thumb=true
		{"Pic/2024-09/Ori/abc.jpg", false},         // clean_ori=false → 只报告
		{"Pic/2024-09/OriTemp/abc.tmp", true},      // clean_temp=true
		{"File/Thumb/abc.png", false},              // clean_file=false
		{"Emoji/emoji-recv/2024-05/Ori/abc.png", false}, // clean_ori=false
		{"Emoji/BaseEmojiSyastems/EmojiSystermResource/😀/png/x.png", false},
		{"Emoji/marketface/123/x.png", false},
		{"Emoji/personal_emoji/Ori/x.png", false},
		{"dataline/.tmp/x.NFC", false}, // clean_dataline_tmp=false（高级默认关）
		{"log/2026-08/x.log", false},   // clean_log=false（高级默认关）
		{"avatar/x.jpg", false},
	}
	for _, c := range gated {
		if got := Whitelisted(ntK(), c.rel, def); got != c.want {
			t.Errorf("Whitelisted(%q) with defaults = %v, want %v", c.rel, got, c.want)
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
		{"/data/nt_qq_xx/nt_data/log/2026-08/x.log", false}, // 已移入白名单政策（clean_log）
		{"/data/nt_qq_xx/nt_data/avatar/x.jpg", false}, // 已移入白名单政策（clean_avatar）
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Thumb/a.png", false},
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Ori/b.jpg", false},
	}
	for _, c := range cases {
		if got := Blacklisted(ntK(), c.abs); got != c.want {
			t.Errorf("Blacklisted(%q) = %v, want %v", c.abs, got, c.want)
		}
	}
}

// AllGatesConfig 全部门控打开的 Config（结构校验用；含高级 opt-in）。
func AllGatesConfig() Config {
	return Config{
		CleanTemp: true, CleanThumb: true, CleanOri: true,
		CleanFile: true, CleanBaseEmoji: true, CleanMarketface: true,
		CleanPersonalEmoji: true, CleanLog: true,
		CleanDatalineTmp: true, CleanAvatar: true,
	}
}
