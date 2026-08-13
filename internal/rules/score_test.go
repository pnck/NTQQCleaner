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

func TestScoreExamples(t *testing.T) {
	cfg := Default()
	now := testutil.Now

	// docs/03 §4 example 1: 7-year-old thumbnail whose Ori original exists.
	// type=10, time=30 (month-archive), redund=20, size=3 → 63.
	e := entry("pic/thumb", "Thumb", "2021-04", "aa", d(2021, 4, 15), 100<<10)
	if got := Score(ntK(), e, idx("aa", true, 2), cfg, now); got != 63 {
		t.Errorf("example 1: got %d want 63", got)
	}
	// example 2: recent thumbnail (31 days, L1) with Ori → 10+5+20+3=38.
	e = entry("pic/thumb", "Thumb", "2026-07", "aa", d(2026, 7, 10), 100<<10)
	if got := Score(ntK(), e, idx("aa", true, 2), cfg, now); got != 38 {
		t.Errorf("example 2: got %d want 38", got)
	}
	// example 3: 3-year-old 100MB original video → 30+30+0+10=70.
	e = entry("video/ori", "Ori", "2023-01", "bb", d(2023, 1, 15), 100<<20)
	if got := Score(ntK(), e, idx("bb", true, 1), cfg, now); got != 70 {
		t.Errorf("example 3: got %d want 70", got)
	}
	// Temp residue: fresh temps still score the minimum (0+0+0+3=3);
	// Tier forces them safe regardless of age.
	e = entry("pic/oritemp", "OriTemp", "2026-08", "cc", now.AddDate(0, 0, -1), 1<<10)
	if got := Score(ntK(), e, idx("cc", false, 1), cfg, now); got != 3 {
		t.Errorf("temp: got %d want 3", got)
	}
}

func TestTierFreshAndTemp(t *testing.T) {
	cfg := Default()
	now := testutil.Now
	// Fresh non-temp → keep, no matter the score.
	e := entry("pic/thumb", "Thumb", "2026-08", "aa", now.AddDate(0, 0, -1), 100<<10)
	if got := Tier(e, 0, cfg, now); got != TierKeep {
		t.Errorf("fresh thumb: got %s want keep", got)
	}
	// Temp is safe even when brand-new.
	e = entry("pic/oritemp", "OriTemp", "2026-08", "bb", now, 1<<10)
	if got := Tier(e, Score(ntK(), e, idx("bb", false, 1), cfg, now), cfg, now); got != TierSafe {
		t.Errorf("fresh temp: got %s want safe", got)
	}
	// ...unless clean_temp is off.
	cfg.CleanTemp = false
	e = entry("pic/oritemp", "OriTemp", "2026-08", "bb", now, 1<<10)
	if got := Tier(e, Score(ntK(), e, idx("bb", false, 1), cfg, now), cfg, now); got != TierKeep {
		t.Errorf("gated temp: got %s want keep", got)
	}
}

func TestTierCategoryGates(t *testing.T) {
	now := testutil.Now
	old := d(2023, 1, 15)
	thumb := entry("pic/thumb", "Thumb", "2023-01", "aa", old, 100<<10)
	ori := entry("pic/ori", "Ori", "2023-01", "bb", old, 100<<10)

	cfg := Default()
	// Default config: thumb participates, ori is report-only.
	if got := Tier(thumb, Score(ntK(), thumb, idx("aa", false, 1), cfg, now), cfg, now); got != TierSuggest {
		t.Errorf("thumb default: got %s want suggest", got)
	}
	if got := Tier(ori, Score(ntK(), ori, idx("bb", true, 1), cfg, now), cfg, now); got != TierKeep {
		t.Errorf("ori default: got %s want keep", got)
	}
	cfg.CleanOri = true
	cfg.Aggressive = true
	if got := Tier(ori, Score(ntK(), ori, idx("bb", true, 1), cfg, now), cfg, now); got != TierCaution {
		t.Errorf("ori enabled+aggressive: got %s want caution", got)
	}
	// File/ is one cautious unit gated by CleanFile — even its thumbnails.
	fthumb := entry("file/thumb", "Thumb", "", "cc", old, 30<<10)
	if got := Tier(fthumb, Score(ntK(), fthumb, idx("cc", false, 1), cfg, now), cfg, now); got != TierKeep {
		t.Errorf("file/thumb default: got %s want keep", got)
	}
	cfg.CleanFile = true
	score := Score(ntK(), fthumb, idx("cc", false, 1), cfg, now)
	if got := Tier(fthumb, score, cfg, now); got != TierSuggest {
		t.Errorf("file/thumb enabled: got %s want suggest (score %d)", got, score)
	}
	// marketface gated by CleanMarketface.
	mf := entry("emoji/marketface", "123", "", "dd", d(2025, 1, 1), 10<<10)
	if got := Tier(mf, Score(ntK(), mf, idx("dd", false, 1), cfg, now), cfg, now); got != TierKeep {
		t.Errorf("marketface default: got %s want keep", got)
	}
	cfg.CleanMarketface = true
	if got := Tier(mf, Score(ntK(), mf, idx("dd", false, 1), cfg, now), cfg, now); got != TierCaution {
		t.Errorf("marketface enabled+aggressive: got %s want caution", got)
	}
	// personal emoji gated by its own flag.
	pe := entry("emoji/personal-emoji/ori", "Ori", "", "ee", d(2024, 1, 1), 10<<10)
	if got := Tier(pe, Score(ntK(), pe, idx("ee", false, 1), cfg, now), cfg, now); got != TierKeep {
		t.Errorf("personal-emoji default: got %s want keep", got)
	}
}

func TestTierAggressive(t *testing.T) {
	now := testutil.Now
	video := entry("video/ori", "Ori", "2023-01", "bb", d(2023, 1, 15), 100<<20) // score 70 → caution
	cfg := Default()
	cfg.CleanOri = true
	score := Score(ntK(), video, idx("bb", true, 1), cfg, now)
	if got := Tier(video, score, cfg, now); got != TierKeep {
		t.Errorf("non-aggressive: got %s want keep", got)
	}
	cfg.Aggressive = true
	if got := Tier(video, score, cfg, now); got != TierCaution {
		t.Errorf("aggressive: got %s want caution", got)
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
