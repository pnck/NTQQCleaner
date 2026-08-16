package clean

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"qqcleaner/internal/classify"
	"qqcleaner/internal/logring"
	"qqcleaner/internal/platform"
	"qqcleaner/internal/qq/impl/nt"
	"qqcleaner/internal/rules"
	"qqcleaner/internal/testutil"
)

// setQQRunning swaps the process guard for the duration of a test.
func setQQRunning(t *testing.T, running bool) {
	t.Helper()
	old := qqRunningFunc
	qqRunningFunc = func() bool { return running }
	t.Cleanup(func() { qqRunningFunc = old })
}

// gateRequest builds a Request that should fail on a safety gate before
// touching any filesystem path.
func gateRequest(force, confirmed bool) Request {
	return Request{
		Files:        []classify.FileEntry{{Path: "/nonexistent/a", Size: 1}},
		AllowedRoots: []string{"/nonexistent"},
		AuditLog:     "/nonexistent/audit.log",
		Force:        force,
		Confirmed:    confirmed,
		K:            ntKN(),
		Config:       rules.Default(),
	}
}

func TestRunRequiresForceAndConfirm(t *testing.T) {
	setQQRunning(t, false)
	if _, err := Run(context.Background(), gateRequest(false, true)); err != ErrNotForced {
		t.Fatalf("Force=false: got %v want ErrNotForced", err)
	}
	if _, err := Run(context.Background(), gateRequest(true, false)); err != ErrNotConfirmed {
		t.Fatalf("Confirmed=false: got %v want ErrNotConfirmed", err)
	}
}

func TestRunRefusesWhileQQRunning(t *testing.T) {
	setQQRunning(t, true)
	_, err := Run(context.Background(), gateRequest(true, true))
	if err == nil || !errors.Is(err, ErrQQRunning) {
		t.Fatalf("got %v want ErrQQRunning", err)
	}
}

// TestRunIgnoreRunning：显式覆盖后 QQ 运行中也执行（产品决策）。
func TestRunIgnoreRunning(t *testing.T) {
	setQQRunning(t, true)
	base := t.TempDir()
	src := filepath.Join(base, "Pic", "2023-01", "Thumb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.png")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := gateRequest(true, true)
	r.Files = []classify.FileEntry{{
		Path: src, Biz: "pic", Sub: "Thumb", Category: "pic/thumb",
		Month: "2023-01", Size: 1, IsThumb: true,
		MTime: testutil.Now.AddDate(-2, 0, 0).Unix(), MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01",
	}}
	r.AllowedRoots = []string{base}
	r.AuditLog = filepath.Join(t.TempDir(), "audit.log")
	r.IgnoreRunning = true
	res, err := Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Fatalf("got %+v", res)
	}
}

// TestRunMovesToBackup verifies the recoverable-move path and the audit
// record (redline: no unrecorded deletion).
func TestRunMovesToBackup(t *testing.T) {
	setQQRunning(t, false)
	base := t.TempDir()
	src := filepath.Join(base, "Pic", "2023-01", "Thumb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.png")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("fake thumbnail")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	audit := filepath.Join(t.TempDir(), "audit.log")

	entry := classify.FileEntry{
		Path: src, Biz: "pic", Sub: "Thumb", Category: "pic/thumb",
		Month: "2023-01", Size: int64(len(content)), IsThumb: true,
		MTime: testutil.Now.AddDate(-2, 0, 0).Unix(), MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01",
	}
	r := Request{
		Files:        []classify.FileEntry{entry},
		AllowedRoots: []string{base},
		BackupDir:    backup,
		Move:         true,
		AuditLog:     audit,
		Force:        true,
		Confirmed:    true,
		K:            ntKN(),
		Config:       rules.Default(),
	}
	res, err := Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Moved != 1 || res.BytesFreed != int64(len(content)) {
		t.Fatalf("got %+v", res)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source still exists after move")
	}
	moved := filepath.Join(backup, filepath.Base(src))
	got, err := os.ReadFile(moved)
	if err != nil || string(got) != string(content) {
		t.Fatalf("backup content mismatch: %v", err)
	}
	// Audit record: move action, original path, backup path.
	line := readAuditLine(t, audit)
	if line.Action != "move" || line.Path != src || line.BackupPath != moved {
		t.Fatalf("audit: %+v", line)
	}
}

// TestRunRemoveLogsPath verifies the delete path without a backup dir:
// audit records the path only（产品决策：不再计算 SHA-256）。
func TestRunRemoveLogsPath(t *testing.T) {
	setQQRunning(t, false)
	base := t.TempDir()
	src := filepath.Join(base, "Pic", "2023-01", "Thumb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.png")
	content := []byte("content")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	audit := filepath.Join(t.TempDir(), "audit.log")
	r := Request{
		Files: []classify.FileEntry{{
			Path: src, Biz: "pic", Sub: "Thumb", Category: "pic/thumb",
			Month: "2023-01", Size: int64(len(content)), IsThumb: true,
			MTime: testutil.Now.AddDate(-2, 0, 0).Unix(), MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01",
		}},
		AllowedRoots: []string{base},
		AuditLog:     audit,
		Force:        true,
		Confirmed:    true,
		K:            ntKN(),
		Config:       rules.Default(),
	}
	res, err := Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Fatalf("got %+v", res)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("file still exists")
	}
	line := readAuditLine(t, audit)
	if line.Action != "remove" || line.Path != src || line.SHA256 != "" {
		t.Fatalf("audit: %+v", line)
	}
}

// TestRunMoveRequiresBackupDir：显式 Move 但未配置备份目录 → 整次运行
// 拒绝（docs/06 §3；GUI 确认对话框同款双保险）。
func TestRunMoveRequiresBackupDir(t *testing.T) {
	setQQRunning(t, false)
	r := gateRequest(true, true)
	r.Move = true
	if _, err := Run(context.Background(), r); err == nil {
		t.Fatal("Run accepted Move without BackupDir")
	}
}

// TestRunRebootDeferredDelete：平台层返回 ErrDeferredReboot（docs/09
// §3.1，Windows 重启删除登记）→ 条目计 reboot 而非 fail，审计记录
// action=reboot，字节不计入 BytesFreed（重启前未真正释放）。
func TestRunRebootDeferredDelete(t *testing.T) {
	setQQRunning(t, false)
	orig := platform.Current()
	defer platform.Install(orig)
	platform.Install(rebaseAdapter{deleteFunc: func(string) error {
		return fmt.Errorf("%w: registered", platform.ErrDeferredReboot)
	}})
	base := t.TempDir()
	src := filepath.Join(base, "Pic", "2023-01", "Thumb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.png")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	audit := filepath.Join(t.TempDir(), "audit.log")
	r := Request{
		Files: []classify.FileEntry{{
			Path: src, Biz: "pic", Sub: "Thumb", Category: "pic/thumb",
			Month: "2023-01", Size: 1, IsThumb: true,
			MTime: testutil.Now.AddDate(-2, 0, 0).Unix(), MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01",
		}},
		AllowedRoots: []string{base},
		AuditLog:     audit,
		Force:        true,
		Confirmed:    true,
		K:            ntKN(),
		Config:       rules.Default(),
	}
	res, err := Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.RebootDeferred != 1 || res.Deleted != 0 || res.Failed != 0 || res.BytesFreed != 0 {
		t.Fatalf("got %+v", res)
	}
	if len(res.Items) != 1 || res.Items[0].Action != "reboot" {
		t.Fatalf("items: %+v", res.Items)
	}
	line := readAuditLine(t, audit)
	if line.Action != "reboot" || line.Path != src {
		t.Fatalf("audit: %+v", line)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("reboot-deferred file should still exist until reboot")
	}
}

// TestRunRebootDeferredMove：Move 路径的 ErrDeferredReboot（备份已生成/
// 重启移动已登记）→ 条目计 reboot、BackupPath 保留。
func TestRunRebootDeferredMove(t *testing.T) {
	setQQRunning(t, false)
	orig := platform.Current()
	defer platform.Install(orig)
	platform.Install(rebaseAdapter{moveFunc: func(src, dst string) error {
		return fmt.Errorf("%w: registered", platform.ErrDeferredReboot)
	}})
	base := t.TempDir()
	src := filepath.Join(base, "Pic", "2023-01", "Thumb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.png")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	r := Request{
		Files: []classify.FileEntry{{
			Path: src, Biz: "pic", Sub: "Thumb", Category: "pic/thumb",
			Month: "2023-01", Size: 1, IsThumb: true,
			MTime: testutil.Now.AddDate(-2, 0, 0).Unix(), MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01",
		}},
		AllowedRoots: []string{base},
		BackupDir:    backup,
		Move:         true,
		Force:        true,
		Confirmed:    true,
		K:            ntKN(),
		Config:       rules.Default(),
	}
	res, err := Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.RebootDeferred != 1 || res.Moved != 0 || res.Failed != 0 {
		t.Fatalf("got %+v", res)
	}
	if len(res.Items) != 1 || res.Items[0].Action != "reboot" ||
		res.Items[0].BackupPath != filepath.Join(backup, filepath.Base(src)) {
		t.Fatalf("items: %+v", res.Items)
	}
}

// TestRunWritesOpsTrace：逐操作 ops 痕迹（docs/09 §3.5）——每个文件
// 的动作实时落盘，被 KILL 时死点精确到单个文件。
func TestRunWritesOpsTrace(t *testing.T) {
	setQQRunning(t, false)
	crash := logring.EnableCrashLog(t.TempDir())
	if crash == "" {
		t.Fatal("EnableCrashLog failed")
	}
	defer logring.Cleanup()
	base := t.TempDir()
	src := filepath.Join(base, "Pic", "2023-01", "Thumb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.png")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Request{
		Files: []classify.FileEntry{{
			Path: src, Biz: "pic", Sub: "Thumb", Category: "pic/thumb",
			Month: "2023-01", Size: 1, IsThumb: true,
			MTime: testutil.Now.AddDate(-2, 0, 0).Unix(), MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01",
		}},
		AllowedRoots: []string{base},
		Force:        true,
		Confirmed:    true,
		K:            ntKN(),
		Config:       rules.Default(),
	}
	if _, err := Run(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(crash)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "clean op: remove "+src) {
		t.Fatalf("ops trace missing remove line:\n%s", data)
	}
}

// rebaseAdapter 是测试用平台适配器：未设置的操作用假实现（返回错误
// 即可，测试只走 DeleteFile/MoveFile），设置后替换对应操作。
type rebaseAdapter struct {
	deleteFunc func(string) error
	moveFunc   func(string, string) error
}

func (a rebaseAdapter) QQProcesses() []string { return nil }
func (a rebaseAdapter) DeleteFile(path string) error {
	if a.deleteFunc != nil {
		return a.deleteFunc(path)
	}
	return os.Remove(path)
}
func (a rebaseAdapter) MoveFile(src, dst string) error {
	if a.moveFunc != nil {
		return a.moveFunc(src, dst)
	}
	return os.Rename(src, dst)
}
func (rebaseAdapter) Reveal(string) error        { return fmt.Errorf("unsupported") }
func (rebaseAdapter) OpenFile(string) error      { return fmt.Errorf("unsupported") }
func (rebaseAdapter) FreezeAnimatedThumbs() bool { return false }
func (rebaseAdapter) ConfirmYesNo(string, string) (bool, error) {
	return false, fmt.Errorf("unsupported")
}

// TestRunWithoutAuditDeletes：审计按需（默认关）——AuditLog 为空时照常
// 删除，但不生成审计文件（docs/06 §3）。
func TestRunWithoutAuditDeletes(t *testing.T) {
	setQQRunning(t, false)
	base := t.TempDir()
	src := filepath.Join(base, "Pic", "2023-01", "Thumb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01_720.png")
	content := []byte("content")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	r := gateRequest(true, true)
	r.Files = []classify.FileEntry{{
		Path: src, Biz: "pic", Sub: "Thumb", Category: "pic/thumb",
		Month: "2023-01", Size: int64(len(content)), IsThumb: true,
		MTime: testutil.Now.AddDate(-2, 0, 0).Unix(), MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01",
	}}
	r.AllowedRoots = []string{base}
	r.AuditLog = ""
	res, err := Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Fatalf("got %+v", res)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("file still exists")
	}
}

// TestRunSkipsBlockedPaths: redline paths are skipped with warnings, never
// deleted, and the run continues.
func TestRunSkipsBlockedPaths(t *testing.T) {
	setQQRunning(t, false)
	base := t.TempDir()
	// A db sidecar inside a whitelisted-looking tree.
	dbPath := filepath.Join(base, "Pic", "2023-01", "Thumb", "x.db-wal")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A path outside the allowed roots.
	outside := filepath.Join(t.TempDir(), "evil.png")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	audit := filepath.Join(t.TempDir(), "audit.log")
	r := Request{
		Files: []classify.FileEntry{
			{Path: dbPath, Biz: "pic", Sub: "Thumb", Category: "pic/thumb", Size: 2},
			{Path: outside, Biz: "pic", Sub: "Thumb", Category: "pic/thumb", Size: 1},
		},
		AllowedRoots: []string{base},
		AuditLog:     audit,
		Force:        true,
		Confirmed:    true,
		K:            ntKN(),
		Config:       rules.Default(),
	}
	res, err := Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 2 || res.Deleted != 0 || res.Moved != 0 {
		t.Fatalf("got %+v", res)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatal("blacklisted path was deleted")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside-root path was deleted")
	}
}

func TestVerifyPath(t *testing.T) {
	// 门控全开：纯结构红线校验（黑名单/根/穿越）。
	cfg := rules.Default()
	cfg.CleanTemp, cfg.CleanThumb, cfg.CleanOri = true, true, true
	cfg.CleanFile, cfg.CleanBaseEmoji, cfg.CleanMarketface, cfg.CleanPersonalEmoji = true, true, true, true
	root := "/data/nt_qq_xx/nt_data"
	cases := []struct {
		path    string
		wantErr bool
	}{
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Thumb/a_720.png", false},
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Ori/a.jpg", false},
		{"/data/nt_qq_xx/nt_data/Pic/2024-09/Thumb/a.db", true},         // db suffix
		{"/data/nt_qq_xx/nt_data/mmkv/mmkv.default", true},              // blocked dir
		{"/data/nt_qq_xx/nt_db/message.db", true},                       // nt_db tree
		{"/etc/passwd", true},                                           // outside roots
		{"/data/nt_qq_xx/nt_data/Pic/a.png", true},                      // too shallow
		{"/data/nt_qq_other/nt_data/Pic/2024-09/Thumb/a_720.png", true}, // different root
	}
	for _, c := range cases {
		err := VerifyPath(ntKN(), c.path, []string{root}, cfg)
		if (err != nil) != c.wantErr {
			t.Errorf("VerifyPath(%q) err=%v wantErr=%v", c.path, err, c.wantErr)
		}
	}

	// 类别门控：CLI 保守默认下 Ori 不可清（只报告）。
	def := rules.Default()
	if err := VerifyPath(ntKN(), "/data/nt_qq_xx/nt_data/Pic/2024-09/Ori/a.jpg", []string{root}, def); err == nil {
		t.Error("VerifyPath accepted Ori with clean_ori=false (CLI default)")
	}
	if err := VerifyPath(ntKN(), "/data/nt_qq_xx/nt_data/Pic/2024-09/Thumb/a_720.png", []string{root}, def); err != nil {
		t.Errorf("VerifyPath rejected Thumb with clean_thumb=true: %v", err)
	}
}

func readAuditLine(t *testing.T, path string) auditEntry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		t.Fatal("audit log empty")
	}
	var e auditEntry
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatalf("bad audit line %q: %v", line, err)
	}
	return e
}

// ntKN 是测试用的 NT 知识实现。
func ntKN() rules.Knowledge { return &nt.NT{} }
