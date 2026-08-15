package legacy

import (
	"os"
	"path/filepath"
	"testing"

	"qqcleaner/internal/qq"
)

// buildLegacyTree 构造旧版布局 fixture（docs/08 §2.4）：
//
//	<root>/10003/msg3.0.db     旧版账号（目录名即 QQ 号）
//	<root>/10004/nt_qq/nt_data Windows NT 实例（必须被 legacy 排除）
//	<root>/All Users、nt_qq     非数字目录（忽略）
func buildLegacyTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(p string) {
		t.Helper()
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string, b []byte) {
		t.Helper()
		mk(filepath.Dir(p))
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "10003", "msg3.0.db"), []byte("x"))
	write(filepath.Join(root, "10003", "Image", "C2C", "$$5SQ.gif"), []byte("yy"))
	mk(filepath.Join(root, "10004", "nt_qq", "nt_data")) // NT 实例，legacy 不得认领
	mk(filepath.Join(root, "All Users"))
	mk(filepath.Join(root, "nt_qq", "global"))
	return root
}

func TestLegacyInstanceDirs(t *testing.T) {
	root := buildLegacyTree(t)
	k := &Legacy{}

	insts, err := k.InstanceDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 1 {
		t.Fatalf("got %d instances want 1: %+v", len(insts), insts)
	}
	inst := insts[0]
	if inst.DirName != "10003" || inst.Hash != "10003" {
		t.Fatalf("instance = %+v want 10003", inst)
	}
	if k.Identify(root, inst) != "10003" {
		t.Fatalf("Identify = %q want 10003", k.Identify(root, inst))
	}
}

func TestLegacyResidues(t *testing.T) {
	root := buildLegacyTree(t)
	res, err := (&Legacy{}).Residues(root, qq.Instance{DirName: "10003", Hash: "10003"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 { // msg3.0.db + Image/
		t.Fatalf("got %d residues want 2: %+v", len(res), res)
	}
	var total int64
	var count int
	for _, r := range res {
		total += r.Size
		count += r.Count
	}
	if total != 3 || count != 2 {
		t.Fatalf("residue total/count = %d/%d want 3/2", total, count)
	}
}

func TestLegacyFailClosed(t *testing.T) {
	k := &Legacy{}
	if k.ScanCapable() {
		t.Fatal("legacy must never be scan-capable (docs/08 §4.1)")
	}
	if k.Whitelisted("Pic/2024-09/Thumb/x.png", qq.AllGates()) {
		t.Fatal("legacy must whitelist nothing")
	}
	if _, _, _, ok := k.ParseFilename("$$5SQ@VGFJQ486@L7Y2~PSK.gif"); ok {
		t.Fatal("legacy must not parse NT filenames")
	}
	if len(k.DBSuffixes()) == 0 || len(k.StateDirs()) == 0 {
		t.Fatal("legacy must keep the version-independent blacklist bottom line")
	}
}

func TestLegacyProbe(t *testing.T) {
	if Probe(buildLegacyTree(t)) == nil {
		t.Fatal("legacy layout should be detected")
	}
	// NT 布局（数字目录含 nt_qq/nt_data）不得被 legacy probe 认领
	ntRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ntRoot, "10004", "nt_qq", "nt_data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if Probe(ntRoot) != nil {
		t.Fatal("NT instances must not match legacy probe")
	}
}
