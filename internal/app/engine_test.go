package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"qqcleaner/internal/rules"
	"qqcleaner/internal/testutil"
)

// captureEmitter records every event and unblocks a waiter channel.
type captureEmitter struct {
	mu   sync.Mutex
	seen []string
	ch   chan string
}

func newCaptureEmitter() *captureEmitter {
	return &captureEmitter{ch: make(chan string, 100)}
}

func (c *captureEmitter) Emit(ev string, _ any) {
	c.mu.Lock()
	c.seen = append(c.seen, ev)
	c.mu.Unlock()
	c.ch <- ev
}

func (c *captureEmitter) waitFor(t *testing.T, ev string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case got := <-c.ch:
			if got == ev {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", ev)
		}
	}
}

func testEngine() *Engine {
	return &Engine{Cfg: rules.Default(), Now: func() time.Time { return testutil.Now }}
}

// TestEngineScanAll is the full integration: fixture → discover → classify
// → reason/关联索引 → aggregate.
func TestEngineScanAll(t *testing.T) {
	f := testutil.BuildQQTree(t)
	out, err := testEngine().ScanAll(context.Background(), f.Root, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Accounts) != 2 {
		t.Fatalf("got %d accounts want 2", len(out.Accounts))
	}
	a, b := out.Accounts[0], out.Accounts[1]
	if a.Hash != testutil.HashA || a.QQNum != testutil.QQA || a.TotalFiles != 10 {
		t.Fatalf("account A: %+v", a)
	}
	if b.Hash != testutil.HashB || b.QQNum != testutil.QQB || b.TotalFiles != 1 {
		t.Fatalf("account B: %+v", b)
	}
	if len(out.Entries) != 11 || len(out.Reasons) != 11 {
		t.Fatalf("entries/reasons: %d/%d want 11/11", len(out.Entries), len(out.Reasons))
	}
}

// TestEngineMinAge excludes fresh files from the outcome entirely.
func TestEngineMinAge(t *testing.T) {
	f := testutil.BuildQQTree(t)
	out, err := testEngine().ScanAll(context.Background(), f.Root, nil, nil, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The 1-day-old Ori is dropped; everything else (≥4 days) stays.
	if out.Accounts[0].TotalFiles != 9 {
		t.Fatalf("got %d files want 9", out.Accounts[0].TotalFiles)
	}
}

// TestDryRunZeroWrites is the redline check (docs/06 §1): a scan must not
// modify a single byte of the data root.
func TestDryRunZeroWrites(t *testing.T) {
	f := testutil.BuildQQTree(t)
	before := snapshotTree(t, f.Root)
	_, err := testEngine().ScanAll(context.Background(), f.Root, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	after := snapshotTree(t, f.Root)
	if len(before) != len(after) {
		t.Fatalf("file count changed: %d → %d", len(before), len(after))
	}
	for p, s := range before {
		if after[p] != s {
			t.Errorf("%s modified by dry-run: %v → %v", p, s, after[p])
		}
	}
}

// snapshotTree records size+mtime for every file under root.
func snapshotTree(t *testing.T, root string) map[string][2]int64 {
	t.Helper()
	snap := map[string][2]int64{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		snap[p] = [2]int64{info.Size(), info.ModTime().UnixNano()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// TestBackendScanQueryPreviewClean exercises the GUI backend end-to-end:
// async scan → events → paged rows → preview resolution → clean.
func TestBackendScanQueryPreviewClean(t *testing.T) {
	f := testutil.BuildQQTree(t)
	cap := newCaptureEmitter()
	backend := NewBackend(filepath.Join(t.TempDir(), "config.yaml"), cap)
	backend.auditPath = filepath.Join(t.TempDir(), "audit.log")

	if err := backend.Scan(ScanOptions{Root: f.Root, MinAgeDays: 0}); err != nil {
		t.Fatal(err)
	}
	cap.waitFor(t, EvDone)

	// Paged rows: 5 thumbs total, page size 3 (filter via expression).
	thumbFilter := Filter{Expr: leaf("thumb", "eq", "true")}
	page, err := backend.QueryRows(PageQuery{Filter: thumbFilter, Page: 1, PageSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 5 || len(page.Rows) != 3 {
		t.Fatalf("page: total=%d rows=%d want 5/3", page.Total, len(page.Rows))
	}
	for _, r := range page.Rows {
		if r.ThumbURL == "" {
			t.Errorf("row %d missing thumbUrl", r.ID)
		}
	}

	// Stats + GetIDs for the select-all-in-filter flow.
	stats, err := backend.GetStats(thumbFilter)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Count != 5 {
		t.Fatalf("thumb stats: got %+v", stats)
	}
	ids, err := backend.GetIDs(Filter{Expr: leaf("temp", "eq", "true")})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("temp ids: got %v want 1", ids)
	}

	// Groups for the tree.
	groups, err := backend.GetGroups(Filter{}, "biz")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 { // pic, emoji, file
		t.Fatalf("biz groups: got %d want 3", len(groups))
	}
	months, err := backend.GetGroups(Filter{}, "month")
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 5 { // 2023-01, 2026-07, 2026-08, 2024-05, 2022-01
		t.Fatalf("month groups: got %d want 5", len(months))
	}

	// 原文件行必须有 OriURL（视频/动图的直接预览入口，此前漏设导致视频无法预览）
	allRows, err := backend.QueryRows(PageQuery{Filter: Filter{}, Page: 1, PageSize: 500})
	if err != nil {
		t.Fatal(err)
	}
	oriRowFound := false
	for _, r := range allRows.Rows {
		if r.Sub == "Ori" {
			oriRowFound = true
			if r.OriURL == "" {
				t.Fatalf("Ori row %d has empty OriURL", r.ID)
			}
		}
	}
	if !oriRowFound {
		t.Fatal("no Ori row in fixture")
	}

	// OriExt：预览端据此分派原文件的播放器。凡有 OriURL 的行必须带
	// OriExt（缩略图行取配对原文件的扩展名，不能用自己的 ext——视频
	// 缩略图行会因此把 mp4 塞进 <img>）；Ori 行则等于自身扩展名。
	for _, r := range allRows.Rows {
		if r.OriURL == "" {
			continue
		}
		if r.OriExt == "" {
			t.Fatalf("row %d has OriURL but empty OriExt", r.ID)
		}
		if r.Sub == "Ori" && r.OriExt != r.Ext {
			t.Fatalf("Ori row %d: OriExt=%q != Ext=%q", r.ID, r.OriExt, r.Ext)
		}
	}

	// Preview: resolve a thumb ID and serve it.
	thumbID := page.Rows[0].ID
	p, err := backend.ResolvePreview(thumbID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("preview path %s: %v", p, err)
	}

	// 索引内每一条目都必须可预览/揭示：keep 级（report-only）条目
	// 如 marketface/personal_emoji（门控关闭时）此前被清理白名单
	// 误拒（"not whitelisted"），「在文件夹中显示」报 Unhandled
	// Promise Rejection。可清性门控只属于 Clean，不属于预览。
	for i := range outcome(backend).Entries {
		if _, err := backend.ResolvePreview(i); err != nil {
			t.Fatalf("entry %d (%s) must resolve for preview/reveal: %v",
				i, outcome(backend).Entries[i].Path, err)
		}
	}

	// Clean the OriTemp residue into a backup dir（选择权在勾选，红线在
	// clean 层逐文件重验）。
	tempIDs := []int{}
	for i, e := range outcome(backend).Entries {
		if e.IsTemp {
			tempIDs = append(tempIDs, i)
		}
	}
	if len(tempIDs) != 1 {
		t.Fatalf("temp ids: got %v want 1", tempIDs)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	res, err := backend.Clean(CleanRequest{IDs: tempIDs, BackupDir: backup, Force: true, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Moved != 1 || res.BytesFreed != 1<<10 {
		t.Fatalf("clean result: %+v", res)
	}
	// The audit log must exist and contain the move.
	audit, err := os.ReadFile(backend.auditPath)
	if err != nil || len(audit) == 0 {
		t.Fatalf("audit log missing: %v", err)
	}
	var entry map[string]any
	if err := json.Unmarshal(audit[:len(audit)-1], &entry); err != nil {
		t.Fatalf("bad audit: %v", err)
	}
	if entry["action"] != "move" {
		t.Fatalf("audit action: %v", entry["action"])
	}
}

// TestMatchOne exercises the condition evaluator (filter editor backend).
func TestMatchOne(t *testing.T) {
	f := testutil.BuildQQTree(t)
	out, err := testEngine().ScanAll(context.Background(), f.Root, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Find a known entry: the 2023-01 thumb (mtime 2023-01-15, 80KB, md5A).
	target := -1
	for id, e := range out.Entries {
		if e.MD5 == testutil.MD5A && e.Sub == "Thumb" && e.Month == "2023-01" {
			target = id
		}
	}
	if target < 0 {
		t.Fatal("fixture thumb not found")
	}
	cases := []struct {
		cond Condition
		want bool
	}{
		{Condition{"biz", "eq", "pic"}, true},
		{Condition{"biz", "ne", "pic"}, false},
		{Condition{"biz", "in", "video,pic"}, true},
		{Condition{"sub", "eq", "Thumb"}, true},
		{Condition{"thumb", "eq", "true"}, true},
		{Condition{"temp", "eq", "true"}, false},
		{Condition{"month", "gte", "2022-01"}, true},
		{Condition{"month", "lt", "2023-01"}, false},
		{Condition{"age", "gte", "1000"}, true}, // ~3.5 years old
		{Condition{"age", "lt", "100"}, false},  // older than 100 days
		{Condition{"size", "lt", "100000"}, true},
		{Condition{"size", "gt", "100000"}, false},
		{Condition{"md5", "contains", "aaaa"}, true},
		{Condition{"md5", "eq", "nope"}, false},
		{Condition{"nonsense", "eq", "x"}, false}, // unknown field fails closed
	}
	for _, c := range cases {
		if got := out.matchOne(target, c.cond); got != c.want {
			t.Errorf("matchOne(%+v) = %v, want %v", c.cond, got, c.want)
		}
	}
	// 布尔树语义：AND/OR/嵌套。
	if !out.evalExpr(target, and(leaf("thumb", "eq", "true"), leaf("age", "gte", "1000"))) {
		t.Error("AND of two true conditions failed")
	}
	if out.evalExpr(target, and(leaf("thumb", "eq", "true"), leaf("age", "lt", "100"))) {
		t.Error("AND of true+false conditions matched")
	}
	// (temp=true OR age>=1000)：该缩略图不满足第一项但满足第二项 → 匹配。
	if !out.evalExpr(target, &Expr{Or: []*Expr{leaf("temp", "eq", "true"), leaf("age", "gte", "1000")}}) {
		t.Error("OR group should match via its second branch")
	}
	// 嵌套：(thumb=true AND age>=1000) OR temp=true
	nested := &Expr{Or: []*Expr{
		and(leaf("thumb", "eq", "true"), leaf("age", "gte", "1000")),
		leaf("temp", "eq", "true"),
	}}
	if !out.evalExpr(target, nested) {
		t.Error("nested OR/AND tree should match")
	}
	// 空组 = 匹配全部
	if !out.evalExpr(target, &Expr{And: []*Expr{}}) {
		t.Error("empty group should match everything")
	}
}

// leaf/and 构造表达式树的测试辅助
func leaf(field, op, value string) *Expr {
	return &Expr{C: &Condition{Field: field, Op: op, Value: value}}
}

func and(subs ...*Expr) *Expr { return &Expr{And: subs} }

func outcome(b *Backend) *Outcome {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.outcome
}

// TestFilterStages exercises take/drop list functions.
func TestFilterStages(t *testing.T) {
	f := testutil.BuildQQTree(t)
	backend := NewBackend("", nil)
	backend.auditPath = filepath.Join(t.TempDir(), "audit.log")
	cap := newCaptureEmitter()
	backend.SetEmitter(cap)
	if err := backend.Scan(ScanOptions{Root: f.Root, MinAgeDays: 0}); err != nil {
		t.Fatal(err)
	}
	cap.waitFor(t, EvDone)

	all, err := backend.GetStats(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if all.Count != 11 {
		t.Fatalf("all: got %d want 11", all.Count)
	}
	// take(3)：大小降序后前 3 条（sort 由 QueryRows 的 Sort 参数控制；
	// GetStats 无法排序，故 take 语义与调用方排序配合——这里只验证截断）
	top3, err := backend.GetStats(Filter{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if top3.Count != 3 {
		t.Fatalf("take(3): got %d want 3", top3.Count)
	}
	// drop(10)：跳过 10 条 → 剩 1
	rest, err := backend.GetStats(Filter{Offset: 10})
	if err != nil {
		t.Fatal(err)
	}
	if rest.Count != 1 {
		t.Fatalf("drop(10): got %d want 1", rest.Count)
	}
	// drop(2)+take(1) → 恰好 1 条
	mid, err := backend.GetStats(Filter{Offset: 2, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if mid.Count != 1 {
		t.Fatalf("drop(2)+take(1): got %d want 1", mid.Count)
	}
	// QueryRows：take(3) 在排序后应用
	page, err := backend.QueryRows(PageQuery{
		Filter:   Filter{Limit: 3},
		Sort:     Sort{Field: "size", Desc: true},
		Page:     1,
		PageSize: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Rows) != 3 {
		t.Fatalf("QueryRows take(3): total=%d rows=%d", page.Total, len(page.Rows))
	}
	// order() 管道自包含：order(size, desc) | take(3) —— 无需外部 Sort
	page, err = backend.QueryRows(PageQuery{
		Filter:   Filter{Orders: []OrderStage{{Field: "size", Desc: true}}, Limit: 3},
		Page:     1,
		PageSize: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Rows) != 3 {
		t.Fatalf("order|take: total=%d rows=%d", page.Total, len(page.Rows))
	}
	// 三行应严格按大小降序（fixture 最大的是 2MB 的 Ori）
	if page.Rows[0].Size < page.Rows[1].Size || page.Rows[1].Size < page.Rows[2].Size {
		t.Fatalf("order(size,desc) not applied: %d %d %d", page.Rows[0].Size, page.Rows[1].Size, page.Rows[2].Size)
	}
}

// TestGetDupes: fixture 中 md5A 出现 3 次（Ori + 两个 Thumb），当前筛选
// 全量时多余副本 = 2 份。
func TestGetDupes(t *testing.T) {
	f := testutil.BuildQQTree(t)
	backend := NewBackend("", nil)
	cap := newCaptureEmitter()
	backend.SetEmitter(cap)
	if err := backend.Scan(ScanOptions{Root: f.Root, MinAgeDays: 0}); err != nil {
		t.Fatal(err)
	}
	cap.waitFor(t, EvDone)

	groups, err := backend.GetDupes(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var md5a *DupGroup
	for i := range groups {
		if groups[i].MD5 == testutil.MD5A {
			md5a = &groups[i]
		}
	}
	if md5a == nil {
		t.Fatalf("md5A group missing: %+v", groups)
	}
	if md5a.Count != 3 || len(md5a.DupIDs) != 2 {
		t.Fatalf("md5A: count=%d dupIds=%d want 3/2", md5a.Count, len(md5a.DupIDs))
	}
	// 保留份必须是 Ori（原图）
	keep := outcome(backend).Entries[md5a.KeepID]
	if keep.Sub != "Ori" {
		t.Fatalf("keeper should be Ori, got %s", keep.Sub)
	}
	// 缩略图筛选下：md5A 组 keeper=Ori（不在筛选内），筛选内的 2 个 Thumb
	// 都是可删副本。
	thumbOnly, err := backend.GetDupes(Filter{Expr: leaf("thumb", "eq", "true")})
	if err != nil {
		t.Fatal(err)
	}
	for i := range thumbOnly {
		if thumbOnly[i].MD5 == testutil.MD5A {
			if len(thumbOnly[i].DupIDs) != 2 {
				t.Fatalf("thumb filter dupIds: got %d want 2", len(thumbOnly[i].DupIDs))
			}
		}
	}
}

// TestBackendCleanSafetyGates: the backend refuses clean without a scan,
// and refuses any clean without force/confirm.
func TestBackendCleanSafetyGates(t *testing.T) {
	backend := NewBackend("", nil)
	if _, err := backend.Clean(CleanRequest{IDs: []int{0}, Force: true, Confirmed: true}); err == nil {
		t.Fatal("expected error: no scan results yet")
	}
	testutil.BuildQQTree(t)
	if _, err := backend.Clean(CleanRequest{IDs: []int{0}, Force: false, Confirmed: true}); err == nil {
		t.Fatal("expected error without force")
	}
}

// TestSetConfigPersists verifies config round-trip.
func TestSetConfigPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	backend := NewBackend(path, nil)
	cfg := backend.GetConfig()
	cfg.CleanOri = true
	if err := backend.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	reloaded := NewBackend(path, nil)
	if !reloaded.GetConfig().CleanOri {
		t.Fatal("CleanOri not persisted")
	}
	// Invalid configs are rejected.
	bad := rules.Default()
	bad.MinFileSizeBytes = -1
	if err := backend.SetConfig(bad); err == nil {
		t.Fatal("invalid config accepted")
	}
}
