package nt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"qqcleaner/internal/discovery"
	"qqcleaner/internal/qq"
)

// 注意：不导入 qqimpl 或 app——两者都（间接）导入 nt，会造成 import
// cycle（qqimpl → nt）。本包（nt）的 init 已注册 nt.Probe，测试二进制
// 加载 nt 包即生效，qq.Detect 可直接命中。Engine 级 Windows 流程的
// 增量逻辑（探测→发现→路径→残留）已由 TestWindowsLayoutDiscover 覆盖，
// 其余管线（classify/rules/clean）与布局无关，由 mac fixture 的既有
// engine 测试覆盖。

// buildWindowsTree 构造 Windows NTQQ 布局 fixture（docs/08 §2.2）：
//
//	<root>/10001/nt_qq/{nt_data,nt_db,nt_temp}   NT 实例（目录名即 QQ 号）
//	<root>/10001/Msg3.0.db, Msg3.0index.db        旧库残留（只统计）
//	<root>/nt_qq/global/nt_data/Login/.10001      全局目录（Windows 位置）
func buildWindowsTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mkAll := func(p string) {
		t.Helper()
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string, b []byte) {
		t.Helper()
		mkAll(filepath.Dir(p))
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ntData := filepath.Join(root, "10001", "nt_qq", "nt_data")
	mkAll(filepath.Join(root, "10001", "nt_qq", "nt_db"))
	mkAll(filepath.Join(root, "10001", "nt_qq", "nt_temp"))
	write(filepath.Join(root, "10001", "Msg3.0.db"), bytes.Repeat([]byte{1}, 4096))
	write(filepath.Join(root, "10001", "Msg3.0index.db"), bytes.Repeat([]byte{2}, 2048))
	write(filepath.Join(root, "nt_qq", "global", "nt_data", "Login", ".10001"), nil)
	// nt_data 缓存与 mac 同构（docs/08 §2.3）
	write(filepath.Join(ntData, "Pic", "2026-07", "Thumb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.jpg"), bytes.Repeat([]byte{3}, 1024))
	return root
}

// withWindowsSpec 把 currentSpec 临时切到 Windows 布局（docs/08 §5）：
// linux 容器上直接验证 Windows 行为的唯一手段。
func withWindowsSpec(t *testing.T) {
	t.Helper()
	old := currentSpec
	currentSpec = windowsSpec
	t.Cleanup(func() { currentSpec = old })
}

func TestWindowsLayoutInstanceDirs(t *testing.T) {
	root := buildWindowsTree(t)
	withWindowsSpec(t)

	k := &NT{}
	insts, err := k.InstanceDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(insts) != 1 {
		t.Fatalf("got %d instances want 1", len(insts))
	}
	inst := insts[0]
	if inst.DirName != "10001" || inst.Hash != "10001" || inst.NtRel != "nt_qq" {
		t.Fatalf("instance = %+v, want DirName=Hash=10001 NtRel=nt_qq", inst)
	}
	// Windows 识别：目录名即 QQ 号（docs/08 §3.4）
	if got := k.Identify(root, inst); got != "10001" {
		t.Fatalf("Identify = %q want 10001", got)
	}
}

func TestWindowsLayoutProbe(t *testing.T) {
	root := buildWindowsTree(t)
	withWindowsSpec(t)
	if Probe(root) == nil {
		t.Fatal("windows layout should be detected as NT")
	}
	// 纯旧版残留数字目录（无 nt_qq/nt_data）不是 NT 实例
	legacyOnly := t.TempDir()
	if err := os.MkdirAll(filepath.Join(legacyOnly, "10009"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyOnly, "10009", "msg3.0.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Probe(legacyOnly) != nil {
		t.Fatal("digit dir without nt_qq/nt_data must not match NT probe (legacy probe owns it)")
	}
}

func TestWindowsLayoutResidues(t *testing.T) {
	root := buildWindowsTree(t)
	withWindowsSpec(t)

	k := &NT{}
	res, err := k.Residues(root, qq.Instance{DirName: "10001", Hash: "10001", NtRel: "nt_qq"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("got %d residues want 2: %+v", len(res), res)
	}
	var total int64
	for _, r := range res {
		total += r.Size
		if filepath.Base(r.Path) == "nt_qq" {
			t.Fatal("nt_qq/ subdir group must not be reported as residue")
		}
	}
	if total != 4096+2048 {
		t.Fatalf("residue total = %d want %d", total, 4096+2048)
	}
}

func TestWindowsLayoutDiscover(t *testing.T) {
	root := buildWindowsTree(t)
	withWindowsSpec(t)

	accs, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) != 1 {
		t.Fatalf("got %d accounts want 1", len(accs))
	}
	a := accs[0]
	if a.Hash != "10001" || a.QQNum != "10001" {
		t.Fatalf("account = %q/%q want 10001/10001", a.Hash, a.QQNum)
	}
	want := filepath.Join(root, "10001", "nt_qq", "nt_data")
	if a.NtData != want {
		t.Fatalf("NtData = %q want %q", a.NtData, want)
	}
	if a.LatestMonth != "2026-07" {
		t.Fatalf("LatestMonth = %q want 2026-07", a.LatestMonth)
	}
	if len(a.LegacyResidues) != 2 || a.LegacyResidues[0].Path != filepath.Join(root, "10001", "Msg3.0.db") {
		t.Fatalf("LegacyResidues = %+v", a.LegacyResidues)
	}
}

// 探测→发现→路径→残留的端到端流程已由 TestWindowsLayoutDiscover 覆盖；
// Engine 的其余管线（classify/rules/clean）与布局无关（见文件头注释）。
