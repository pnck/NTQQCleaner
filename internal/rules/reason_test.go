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

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 12, 0, 0, 0, time.Local) }

// TestReason 覆盖 reason 标签的三类关联情况（docs/03 §4）：
// ① 字节级重复（重复出现，contentCount ≥ 2）
// ② 有对应原文件（原图仍在，缩略图行）
// ③ 有对应缩略图（有缩略图，原文件行）——Ori 不得因同名缩略图被误标
//    「重复出现」（回归：用户反馈的 bug）。
func TestReason(t *testing.T) {
	now := testutil.Now
	cases := []struct {
		name                       string
		e                          classify.FileEntry
		hasOri, hasThumb, dupCount bool
		want                       string
	}{
		{"temp", entry("pic/oritemp", "OriTemp", "2026-08", "aa", now, 1<<10), false, false, false, "下载中断残留"},
		{"thumb with ori", entry("pic/thumb", "Thumb", "2023-01", "aa", d(2023, 1, 15), 10), true, false, false, "缩略图；原图仍在"},
		{"thumb with ori and dup", entry("pic/thumb", "Thumb", "2023-01", "aa", d(2023, 1, 15), 10), true, false, true, "缩略图；原图仍在；重复出现"},
		{"thumb no ori", entry("pic/thumb", "Thumb", "2023-01", "bb", d(2023, 1, 15), 10), false, false, false, "缩略图"},
		{"thumb dup only", entry("pic/thumb", "Thumb", "2023-01", "bb", d(2023, 1, 15), 10), false, false, true, "缩略图；重复出现"},
		{"ori with thumb", entry("pic/ori", "Ori", "2023-01", "cc", d(2023, 1, 15), 10), false, true, false, "原图/原文件；有缩略图"},
		{"ori with thumb and dup", entry("pic/ori", "Ori", "2023-01", "cc", d(2023, 1, 15), 10), false, true, true, "原图/原文件；有缩略图；重复出现"},
		{"ori alone", entry("pic/ori", "Ori", "2023-01", "dd", d(2023, 1, 15), 10), false, false, false, "原图/原文件"},
		{"dup only", entry("other", "x", "", "", now, 10), false, false, true, "重复出现"},
		{"marketface", entry("emoji/marketface", "123", "", "ee", d(2025, 1, 1), 10), false, false, false, "表情包"},
		{"personal emoji", entry("emoji/personal-emoji/ori", "Ori", "", "ff", d(2024, 1, 1), 10), false, false, false, "个人表情"},
		{"fallback", entry("other", "x", "", "", now, 10), false, false, false, "缓存文件"},
		{"no md5", entry("pic/thumb", "Thumb", "2023-01", "", d(2023, 1, 15), 10), false, false, false, "缩略图"},
	}
	for _, c := range cases {
		got := Reason(c.e, c.hasOri, c.hasThumb, map[bool]int{false: 0, true: 2}[c.dupCount])
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

// ntK 是测试用的 NT 知识实现。
func ntK() Knowledge { return &nt.NT{} }
