package qq_test

import (
	"os"
	"path/filepath"
	"testing"

	"qqcleaner/internal/qq"
	_ "qqcleaner/internal/qqimpl" // 注册 probe
	"qqcleaner/internal/testutil"
)

// TestDetectNT：nt 布局 fixture 命中 NT 实现。
func TestDetectNT(t *testing.T) {
	f := testutil.BuildQQTree(t)
	k := qq.Detect(f.Root)
	if k.Name() != "nt" || !k.ScanCapable() {
		t.Fatalf("got %s (capable=%v), want nt", k.Name(), k.ScanCapable())
	}
}

// TestDetectGeneric：未知布局 fail-closed 到 generic。
func TestDetectGeneric(t *testing.T) {
	k := qq.Detect(t.TempDir())
	if k.Name() != "generic" || k.ScanCapable() {
		t.Fatalf("got %s (capable=%v), want generic incapable", k.Name(), k.ScanCapable())
	}
	// generic 白名单全拒绝，黑名单底线仍在
	if k.Whitelisted("Pic/2024-09/Thumb/x.png", qq.Gates{}) {
		t.Error("generic should whitelist nothing")
	}
	if len(k.DBSuffixes()) == 0 {
		t.Error("generic should keep db-suffix blacklist")
	}
}

// TestDetectPrefersCapable：新旧布局并存（nt_qq_* 与数字目录+msg3.0.db）
// 时，ScanCapable 的 nt 实现必须胜出——与 probe 注册顺序无关。
func TestDetectPrefersCapable(t *testing.T) {
	f := testutil.BuildQQTree(t)
	// 追加旧版残留：数字账号目录 + msg3.0.db
	legacyDir := filepath.Join(f.Root, "10003")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "msg3.0.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	k := qq.Detect(f.Root)
	if k.Name() != "nt" || !k.ScanCapable() {
		t.Fatalf("got %s (capable=%v), want nt — capable layouts must win over legacy residue", k.Name(), k.ScanCapable())
	}
}

// TestDetectLegacyOnly：旧版布局（数字目录+msg3.0.db）被 legacy 兼容层
// 识别（docs/08 §3.3）：可报告占用，但 ScanCapable=false——扫描/清理
// fail-closed。未知布局仍回退 generic。
func TestDetectLegacyOnly(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "10003")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "msg3.0.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	k := qq.Detect(root)
	if k.Name() != "legacy" || k.ScanCapable() {
		t.Fatalf("got %s (capable=%v), want legacy incapable (recognized but fail-closed)", k.Name(), k.ScanCapable())
	}
	// legacy 白名单全拒绝，黑名单底线仍在
	if k.Whitelisted("Pic/2024-09/Thumb/x.png", qq.Gates{}) {
		t.Error("legacy should whitelist nothing")
	}
	if len(k.DBSuffixes()) == 0 {
		t.Error("legacy should keep db-suffix blacklist")
	}
}
