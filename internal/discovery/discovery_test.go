package discovery

import (
	"os"
	"path/filepath"
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
	qq, err := IdentifyFromMmkv(f.Root, "deadbeefdeadbeefdeadbeefdeadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if qq != "" {
		t.Fatalf("unknown hash: got %q want empty", qq)
	}
}

func TestIdentifyFromUnitedConfig(t *testing.T) {
	f := testutil.BuildQQTree(t)
	qq, err := IdentifyFromUnitedConfig(f.NtDataA)
	if err != nil {
		t.Fatal(err)
	}
	if qq != testutil.QQA {
		t.Fatalf("got %q want %q", qq, testutil.QQA)
	}
	// Account B only has "000" → no QQ number (mmkv is the fallback source).
	qq, err = IdentifyFromUnitedConfig(f.NtDataB)
	if err != nil {
		t.Fatal(err)
	}
	if qq != "" {
		t.Fatalf("got %q want empty", qq)
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
	for _, qq := range got {
		if !want[qq] {
			t.Fatalf("unexpected account %q", qq)
		}
	}
}

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
