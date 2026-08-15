package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "qqcleaner/internal/qqimpl" // 注册 probe（nt 布局探测）
	"qqcleaner/internal/testutil"
)

func TestDiscover(t *testing.T) {
	f := testutil.BuildQQTree(t)
	accs, err := Discover(f.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) != 2 {
		t.Fatalf("got %d accounts want 2", len(accs))
	}
	// Newest Pic activity first (docs/02 §6).
	a, b := accs[0], accs[1]
	if a.Hash != testutil.HashA || b.Hash != testutil.HashB {
		t.Fatalf("ordering: got [%s %s] want [A B]", a.Hash, b.Hash)
	}
	if a.QQNum != testutil.QQA || a.LatestMonth != "2026-08" {
		t.Fatalf("account A: qq=%q month=%q", a.QQNum, a.LatestMonth)
	}
	if b.QQNum != testutil.QQB || b.LatestMonth != "2022-01" {
		t.Fatalf("account B: qq=%q month=%q", b.QQNum, b.LatestMonth)
	}
	if a.NtData != f.NtDataA || a.NtDb == "" {
		t.Fatalf("paths: ntData=%q ntDb=%q", a.NtData, a.NtDb)
	}
}

func TestDiscoverIgnoresNonInstances(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nt_qq_short"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "nt_qq_"+testutil.HashA), 0o755); err != nil {
		t.Fatal(err)
	}
	accs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) != 1 || accs[0].Hash != testutil.HashA {
		t.Fatalf("got %+v", accs)
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	if _, err := Discover(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing root")
	}
}

// TestLegacySummary：旧版根生成占用报告（含逐条与合计）；NT 根与非
// QQ 根返回空串（docs/08 §3.5，CLI/GUI 共用）。
func TestLegacySummary(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "10003"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "10003", "msg3.0.db"), []byte("xy"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := LegacySummary(root)
	if s == "" {
		t.Fatal("legacy root should produce a summary")
	}
	if !strings.Contains(s, "10003") || !strings.Contains(s, "msg3.0.db") || !strings.Contains(s, "合计") {
		t.Fatalf("summary missing parts: %q", s)
	}
	// NT 根与非 QQ 根不产生旧版摘要
	f := testutil.BuildQQTree(t)
	if LegacySummary(f.Root) != "" {
		t.Fatal("NT root must not produce a legacy summary")
	}
	if LegacySummary(t.TempDir()) != "" {
		t.Fatal("non-QQ root must not produce a legacy summary")
	}
}
