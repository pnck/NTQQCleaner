package classify

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"qqcleaner/internal/testutil"
)

func TestScanClassifies(t *testing.T) {
	f := testutil.BuildQQTree(t)
	entries, err := Scan(context.Background(), f.NtDataA, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 {
		t.Fatalf("got %d entries want 10", len(entries))
	}
	by := map[string]FileEntry{}
	for _, e := range entries {
		by[rel(t, f.NtDataA, e.Path)] = e
	}

	cases := []struct {
		rel                                          string
		biz, sub, category, month, md5, sizeTag, ext string
		thumb, tmp                                   bool
	}{
		{"Pic/2023-01/Ori/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01.jpg", "pic", "Ori", "pic/ori", "2023-01", testutil.MD5A, "", "jpg", false, false},
		{"Pic/2023-01/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.jpg", "pic", "Thumb", "pic/thumb", "2023-01", testutil.MD5A, "720", "jpg", true, false},
		{"Pic/2026-07/OriTemp/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa06.tmp", "pic", "OriTemp", "pic/oritemp", "2026-07", testutil.MD5Temp, "", "tmp", false, true},
		{"Emoji/emoji-recv/2024-05/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa02_720.png", "emoji", "Thumb", "emoji/emoji-recv/thumb", "2024-05", testutil.MD5B, "720", "png", true, false},
		{"Emoji/marketface/123/x.png", "emoji", "123", "emoji/marketface", "", "", "", "png", false, false},
		{"Emoji/personal_emoji/Ori/my.png", "emoji", "Ori", "emoji/personal-emoji/ori", "", "", "", "png", false, false},
		{"File/Thumb/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa03.png", "file", "Thumb", "file/thumb", "", testutil.MD5C, "", "png", true, false},
		{"File/file_assistant/doc.txt", "file", "file_assistant", "file/file_assistant", "", "", "", "txt", false, false},
	}
	for _, c := range cases {
		e, ok := by[c.rel]
		if !ok {
			t.Fatalf("missing entry %s (have %v)", c.rel, keys(by))
		}
		if e.Biz != c.biz || e.Sub != c.sub || e.Category != c.category || e.Month != c.month ||
			e.MD5 != c.md5 || e.SizeTag != c.sizeTag || e.Ext != c.ext || e.IsThumb != c.thumb || e.IsTemp != c.tmp {
			t.Errorf("%s:\n got  biz=%s sub=%s cat=%s month=%s md5=%s tag=%s ext=%s thumb=%v temp=%v\n want biz=%s sub=%s cat=%s month=%s md5=%s tag=%s ext=%s thumb=%v temp=%v",
				c.rel, e.Biz, e.Sub, e.Category, e.Month, e.MD5, e.SizeTag, e.Ext, e.IsThumb, e.IsTemp,
				c.biz, c.sub, c.category, c.month, c.md5, c.sizeTag, c.ext, c.thumb, c.tmp)
		}
	}
	// State/config dirs and encrypted DBs must never appear (redline).
	for _, e := range entries {
		if strings.Contains(e.Path, "mmkv") || strings.Contains(e.Path, "nt_db") {
			t.Errorf("blacklisted path leaked into scan: %s", e.Path)
		}
	}
}

func TestScanMinSizeAndOnlyBizs(t *testing.T) {
	f := testutil.BuildQQTree(t)
	entries, err := Scan(context.Background(), f.NtDataA, Options{MinSize: 100 << 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Size < 100<<10 {
			t.Errorf("entry below min size: %s (%d)", e.Path, e.Size)
		}
	}
	entries, err = Scan(context.Background(), f.NtDataA, Options{OnlyBizs: []string{"File"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("File-only scan: got %d entries want 2", len(entries))
	}
	for _, e := range entries {
		if e.Biz != "file" {
			t.Errorf("unexpected biz %s in File-only scan", e.Biz)
		}
	}
}

func TestScanCancel(t *testing.T) {
	f := testutil.BuildQQTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, f.NtDataA, Options{}); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func rel(t *testing.T, base, abs string) string {
	t.Helper()
	r, err := filepath.Rel(base, abs)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(r)
}

func keys(m map[string]FileEntry) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
