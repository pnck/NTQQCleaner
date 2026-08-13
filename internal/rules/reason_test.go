package rules

import (
	"testing"
	"time"

	"qqcleaner/internal/classify"
	"qqcleaner/internal/qq/impl/nt"
	"qqcleaner/internal/testutil"
)

func entry(category, sub, month, md5 string, mtime time.Time, size int64) classify.FileEntry {
	return classify.FileEntry{
		Category: category,
		Sub:      sub,
		Month:    month,
		MD5:      md5,
		MTime:    mtime.Unix(),
		Size:     size,
		IsThumb:  sub == "Thumb",
		IsTemp:   sub == "OriTemp" || sub == "ThumbTemp",
	}
}

func idx(md5 string, hasOri bool, count int) MD5Index {
	return MD5Index{md5: {HasOri: hasOri, Count: count}}
}

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 12, 0, 0, 0, time.Local) }

// TestReason 覆盖 reason 短标签的各类别组合（docs/03 §5 的说明标签）。
func TestReason(t *testing.T) {
	now := testutil.Now
	cases := []struct {
		name string
		e    classify.FileEntry
		idx  MD5Index
		want string
	}{
		{"temp", entry("pic/oritemp", "OriTemp", "2026-08", "aa", now, 1<<10), nil, "下载中断残留"},
		{"thumb with ori", entry("pic/thumb", "Thumb", "2023-01", "aa", d(2023, 1, 15), 10), idx("aa", true, 2), "缩略图；原图仍在"},
		{"thumb no ori", entry("pic/thumb", "Thumb", "2023-01", "bb", d(2023, 1, 15), 10), idx("bb", false, 1), "缩略图"},
		{"ori", entry("pic/ori", "Ori", "2023-01", "cc", d(2023, 1, 15), 10), idx("cc", true, 1), "原图/原文件"},
		{"marketface", entry("emoji/marketface", "123", "", "dd", d(2025, 1, 1), 10), nil, "表情包"},
		{"personal emoji", entry("emoji/personal-emoji/ori", "Ori", "", "ee", d(2024, 1, 1), 10), nil, "个人表情"},
		{"fallback", entry("other", "x", "", "", now, 10), nil, "缓存文件"},
		{"no md5", entry("pic/thumb", "Thumb", "2023-01", "", d(2023, 1, 15), 10), nil, "缩略图"},
	}
	for _, c := range cases {
		if got := Reason(c.e, c.idx); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestBuildMD5Index(t *testing.T) {
	now := testutil.Now
	entries := []classify.FileEntry{
		entry("pic/ori", "Ori", "2023-01", "aa", d(2023, 1, 15), 10),
		entry("pic/thumb", "Thumb", "2023-01", "aa", d(2023, 1, 15), 10),
		entry("pic/thumb", "Thumb", "2026-07", "aa", d(2026, 7, 10), 10),
		entry("pic/thumb", "Thumb", "2026-07", "bb", d(2026, 7, 10), 10),
		entry("emoji/personal-emoji/ori", "Ori", "", "", now, 10), // no md5
	}
	i := BuildMD5Index(entries)
	if v := i["aa"]; !v.HasOri || v.Count != 3 {
		t.Errorf("aa: got %+v want HasOri Count=3", v)
	}
	if v := i["bb"]; v.HasOri || v.Count != 1 {
		t.Errorf("bb: got %+v want Count=1", v)
	}
	if _, ok := i[""]; ok {
		t.Error("empty md5 indexed")
	}
}

// ntK 是测试用的 NT 知识实现。
func ntK() Knowledge { return &nt.NT{} }
