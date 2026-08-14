package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if a.Hash != testutil.HashA || a.QQNum != testutil.QQA || a.TotalFiles != 15 {
		t.Fatalf("account A: %+v", a)
	}
	if b.Hash != testutil.HashB || b.QQNum != testutil.QQB || b.TotalFiles != 1 {
		t.Fatalf("account B: %+v", b)
	}
	if len(out.Entries) != 16 || len(out.Reasons) != 16 {
		t.Fatalf("entries/reasons: %d/%d want 16/16", len(out.Entries), len(out.Reasons))
	}
	// 已知可清结构必须进入索引：dataline/.tmp 的 NFC 传输残留（clean_temp）
	// 与 log/log-cache 运行日志（clean_log）。
	var nfc, logs int
	for _, e := range out.Entries {
		if strings.HasSuffix(e.Path, ".NFC") {
			nfc++
			if e.Sub != "tmp" || e.Category != "dataline/tmp" {
				t.Fatalf("NFC residue classified wrong: sub=%q category=%q", e.Sub, e.Category)
			}
		}
		if e.Biz == "log" || e.Biz == "log-cache" {
			logs++
		}
	}
	if nfc != 1 || logs != 2 {
		t.Fatalf("NFC/log entries: nfc=%d logs=%d want 1/2", nfc, logs)
	}
	// 二次扫描生效的直接证据：A 账号 6 个大小冲突候选（2×MD5A 80KB、
	// MD5F/MD5G 60KB、x.png/my.png 10KB）；B 账号 1 个（MD5D 60KB）。
	if a.HashedFiles != 6 || b.HashedFiles != 1 {
		t.Fatalf("hashedFiles: A=%d B=%d want 6/1", a.HashedFiles, b.HashedFiles)
	}
}

// TestContentHashAndReasons：二次扫描的内容哈希与三类关联标签。
// fixture 中 md5F/md5G/md5D（60KB）字节相同、名字不同——跨目录跨账号的
// 真冗余；marketface/x.png 与 personal_emoji/my.png 同大小（10KB）但内容
// 不同，不得误判。
func TestContentHashAndReasons(t *testing.T) {
	f := testutil.BuildQQTree(t)
	out, err := testEngine().ScanAll(context.Background(), f.Root, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	byMD5 := map[string]int{}
	for id, e := range out.Entries {
		if e.MD5 != "" {
			byMD5[e.MD5] = id
		}
	}
	// 60KB 三份：md5F/md5G/md5D 必须同哈希且计入内容组。
	hF := out.Entries[byMD5[testutil.MD5F]].ContentHash
	if hF == "" {
		t.Fatal("md5F must be hashed (size collision)")
	}
	for _, m := range []string{testutil.MD5G, testutil.MD5D} {
		if out.Entries[byMD5[m]].ContentHash != hF {
			t.Errorf("%s: hash %q want %q", m, out.Entries[byMD5[m]].ContentHash, hF)
		}
	}
	if got := len(out.ContentIndex[hF]); got != 3 {
		t.Fatalf("60KB group size: got %d want 3", got)
	}
	// 两个 MD5A 缩略图（80KB）字节相同 → 同组；MD5A 的 Ori 是 2MB（大小
	// 唯一）→ 不得计算哈希。
	oriID := -1
	thumbs := 0
	for id, e := range out.Entries {
		if e.MD5 != testutil.MD5A {
			continue
		}
		if e.Sub == "Ori" {
			oriID = id
			if e.ContentHash != "" {
				t.Error("unique-size Ori (2MB) must not be hashed")
			}
		} else {
			thumbs++
			if e.ContentHash == "" {
				t.Errorf("MD5A thumb %d not hashed", id)
			}
		}
	}
	if oriID < 0 || thumbs != 2 {
		t.Fatalf("MD5A entries: ori=%d thumbs=%d want ori+2", oriID, thumbs)
	}
	// 同大小不同内容（10KB 对）必须哈希不同。
	xh, myh := "", ""
	for _, e := range out.Entries {
		if filepath.Base(e.Path) == "x.png" {
			xh = e.ContentHash
		}
		if filepath.Base(e.Path) == "my.png" {
			myh = e.ContentHash
		}
	}
	if xh == "" || myh == "" {
		t.Fatal("10KB pair must both be hashed (size collision)")
	}
	if xh == myh {
		t.Error("same-size different-content files must hash differently")
	}

	// 关联标签（用户要求的三种情况区分）：
	// ① Ori 有同名缩略图 → 「有缩略图」，绝不能再标「重复出现」。
	oriReason := out.Reasons[oriID]
	if !strings.Contains(oriReason, "有缩略图") || strings.Contains(oriReason, "重复出现") {
		t.Fatalf("Ori reason: got %q want 原图/原文件；有缩略图（不得含 重复出现）", oriReason)
	}
	// ② 缩略图有原文件 → 「原图仍在」；80KB 组两份内容相同 → 「重复出现」。
	for id, e := range out.Entries {
		if e.MD5 == testutil.MD5A && e.IsThumb {
			r := out.Reasons[id]
			if !strings.Contains(r, "原图仍在") || !strings.Contains(r, "重复出现") {
				t.Errorf("MD5A thumb %d reason: got %q want 缩略图；原图仍在；重复出现", id, r)
			}
		}
	}
	// ③ 60KB 组：真冗余副本 → 「重复出现」，且无配对标签。
	for _, m := range []string{testutil.MD5F, testutil.MD5G, testutil.MD5D} {
		r := out.Reasons[byMD5[m]]
		if !strings.Contains(r, "重复出现") || strings.Contains(r, "原图仍在") || strings.Contains(r, "有缩略图") {
			t.Errorf("%s reason: got %q want 缩略图；重复出现", m, r)
		}
	}
	// 唯一大小文件不得有「重复出现」。
	for id, e := range out.Entries {
		if e.ContentHash == "" && strings.Contains(out.Reasons[id], "重复出现") {
			t.Errorf("entry %d has no content hash but reason says 重复出现", id)
		}
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
	if out.Accounts[0].TotalFiles != 14 {
		t.Fatalf("got %d files want 14", out.Accounts[0].TotalFiles)
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

	// Paged rows: 7 thumbs total, page size 3 (filter via expression).
	thumbFilter := Filter{Expr: leaf("thumb", "eq", "true")}
	page, err := backend.QueryRows(PageQuery{Filter: thumbFilter, Page: 1, PageSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 7 || len(page.Rows) != 3 {
		t.Fatalf("page: total=%d rows=%d want 7/3", page.Total, len(page.Rows))
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
	if stats.Count != 7 {
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
	if len(groups) != 6 { // pic, emoji, file, dataline, log, log-cache
		t.Fatalf("biz groups: got %d want 6", len(groups))
	}
	for i := 1; i < len(groups); i++ {
		if groups[i].Size > groups[i-1].Size {
			t.Errorf("biz groups not sorted by size desc: %v then %v", groups[i-1].Key, groups[i].Key)
		}
	}
	months, err := backend.GetGroups(Filter{}, "month")
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 5 { // 2023-01, 2026-07, 2026-08, 2024-05, 2022-01
		t.Fatalf("month groups: got %d want 5", len(months))
	}
	// 月份按时间倒序（新→旧），而非按大小排序。
	wantMonths := []string{"2026-08", "2026-07", "2024-05", "2023-01", "2022-01"}
	for i, want := range wantMonths {
		if months[i].Key != want {
			t.Fatalf("month order: got %v want %v", months[i].Key, wantMonths)
		}
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
		// month 按可计算时间比较（当月时间戳），非字符串序
		{Condition{"month", "eq", "2023-01"}, true},
		{Condition{"month", "ne", "2023-02"}, true},
		{Condition{"month", "in", "2022-12,2023-01"}, true},
		{Condition{"month", "in", "2022-12,2022-11"}, false},
		{Condition{"month", "gt", "2022-12"}, true},
		{Condition{"month", "gte", "2023-01"}, true},
		{Condition{"month", "lt", "2023-02"}, true},
		{Condition{"month", "lte", "2022-12"}, false},
		{Condition{"month", "contains", "2023"}, true},
		{Condition{"month", "contains", "2024"}, false},
		{Condition{"month", "gt", "2023-13"}, false}, // 非法月份 fail closed
		{Condition{"month", "gt", "junk"}, false},    // 不可解析 fail closed
		{Condition{"age", "gte", "1000"}, true},      // ~3.5 years old
		{Condition{"age", "lt", "100"}, false},       // older than 100 days
		{Condition{"size", "lt", "100000"}, true},
		{Condition{"size", "gt", "100000"}, false},
		{Condition{"size", "in", "70000,81920"}, true}, // 80KB = 81920
		{Condition{"size", "in", "70000,80000"}, false},
		{Condition{"fileId", "contains", "aaaa"}, true},
		{Condition{"fileId", "eq", "nope"}, false},
		{Condition{"nonsense", "eq", "x"}, false}, // unknown field fails closed
	}
	for _, c := range cases {
		if got := out.matchOne(target, c.cond); got != c.want {
			t.Errorf("matchOne(%+v) = %v, want %v", c.cond, got, c.want)
		}
	}
	// age 的 in 按真实天数精确比较（Now 注入固定，按目标文件动态计算，
	// 不硬编码天数魔法值）。
	ageDays := int64(out.Now.Sub(time.Unix(out.Entries[target].MTime, 0)).Hours() / 24)
	if !out.matchOne(target, Condition{"age", "in", fmt.Sprintf("999,%d", ageDays)}) {
		t.Errorf("age in exact value %d failed", ageDays)
	}
	if out.matchOne(target, Condition{"age", "in", "999,1000"}) {
		t.Error("age in without the exact value matched")
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
	if all.Count != 16 {
		t.Fatalf("all: got %d want 16", all.Count)
	}
	// take(3)：截断语义（GetStats 无 UI 排序，take 按自然顺序截断）
	top3, err := backend.GetStats(Filter{Stages: []Stage{{Kind: "take", N: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if top3.Count != 3 {
		t.Fatalf("take(3): got %d want 3", top3.Count)
	}
	// drop(10)：跳过 10 条 → 剩 3
	rest, err := backend.GetStats(Filter{Stages: []Stage{{Kind: "drop", N: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	if rest.Count != 6 {
		t.Fatalf("drop(10): got %d want 6", rest.Count)
	}
	// drop(2)+take(1) → 恰好 1 条
	mid, err := backend.GetStats(Filter{Stages: []Stage{{Kind: "drop", N: 2}, {Kind: "take", N: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if mid.Count != 1 {
		t.Fatalf("drop(2)+take(1): got %d want 1", mid.Count)
	}
	// QueryRows：take(3) 在 UI 排序后应用
	page, err := backend.QueryRows(PageQuery{
		Filter:   Filter{Stages: []Stage{{Kind: "take", N: 3}}},
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
		Filter:   Filter{Stages: []Stage{{Kind: "order", Field: "size", Desc: true}, {Kind: "take", N: 3}}},
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

// TestGetDupes：去重组按字节级内容哈希分组（不再按文件名 md5）。
// fixture 期望两组：
//   - 60KB 组：md5F/md5G/md5D 三份字节相同（跨账号、不同名），
//     keeper = md5F（mtime 最新），可删副本 2 份；
//   - 80KB 组：md5A 的两个缩略图字节相同，keeper = 2026-07 那份，可删 1 份；
//   - 同名配对不构成重复：md5A 的 Ori（2MB）与缩略图字节不同，不入组；
//   - 同大小不同内容（10KB 对）不入组。
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
	if len(groups) != 2 {
		t.Fatalf("groups: got %d want 2: %+v", len(groups), groups)
	}
	var g60, g80 *DupGroup
	for i := range groups {
		if groups[i].Count == 3 {
			g60 = &groups[i]
		}
		if groups[i].Count == 2 {
			g80 = &groups[i]
		}
	}
	if g60 == nil || g80 == nil {
		t.Fatalf("want one 3-copy and one 2-copy group, got %+v", groups)
	}
	if len(g60.DupIDs) != 2 || len(g80.DupIDs) != 1 {
		t.Fatalf("dupIds: g60=%d g80=%d want 2/1", len(g60.DupIDs), len(g80.DupIDs))
	}
	// keeper：60KB 组 = md5F（2026-07 最新）；80KB 组 = 2026-07 的 md5A 缩略图。
	if k := outcome(backend).Entries[g60.KeepID]; k.MD5 != testutil.MD5F {
		t.Fatalf("60KB keeper: got %s want md5F", k.MD5)
	}
	k80 := outcome(backend).Entries[g80.KeepID]
	if k80.MD5 != testutil.MD5A || k80.Month != "2026-07" {
		t.Fatalf("80KB keeper: got %s/%s want md5A/2026-07", k80.MD5, k80.Month)
	}
	// 组的 Hash 必须是 SHA-256（64 hex），且与条目内容哈希一致。
	if len(g60.Hash) != 64 {
		t.Fatalf("group hash not sha256: %q", g60.Hash)
	}
	if g60.Hash != outcome(backend).Entries[g60.KeepID].ContentHash {
		t.Fatal("group hash must equal keeper content hash")
	}

	// 缩略图筛选下：两组的所有副本都在筛选内，可删份数不变。
	thumbOnly, err := backend.GetDupes(Filter{Expr: leaf("thumb", "eq", "true")})
	if err != nil {
		t.Fatal(err)
	}
	for i := range thumbOnly {
		switch thumbOnly[i].Count {
		case 3:
			if len(thumbOnly[i].DupIDs) != 2 {
				t.Fatalf("thumb filter g60 dupIds: got %d want 2", len(thumbOnly[i].DupIDs))
			}
		case 2:
			if len(thumbOnly[i].DupIDs) != 1 {
				t.Fatalf("thumb filter g80 dupIds: got %d want 1", len(thumbOnly[i].DupIDs))
			}
		}
	}

	// contentHash 筛选字段：按组哈希圈定该组全部副本。
	byHash := Filter{Expr: leaf("contentHash", "eq", g60.Hash)}
	stats, err := backend.GetStats(byHash)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Count != 3 {
		t.Fatalf("contentHash filter: got %d want 3", stats.Count)
	}
}

// TestSelectAssociated：select() 管道的三种关联展开（docs/04 §3）。
//   - ori  ：缩略图 → 其原文件；原文件保留自身；无配对移除
//   - thumb：原文件 → 其全部缩略图
//   - dup  ：展开为内容哈希组（字节级相同，含自身）
func TestSelectAssociated(t *testing.T) {
	f := testutil.BuildQQTree(t)
	out, err := testEngine().ScanAll(context.Background(), f.Root, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	oriID := -1
	thumbs := []int{}
	dupF := -1
	market := -1
	for id, e := range out.Entries {
		switch {
		case e.MD5 == testutil.MD5A && e.Sub == "Ori":
			oriID = id
		case e.MD5 == testutil.MD5A && e.IsThumb:
			thumbs = append(thumbs, id)
		case e.MD5 == testutil.MD5F:
			dupF = id
		case filepath.Base(e.Path) == "x.png":
			market = id
		}
	}
	if oriID < 0 || len(thumbs) != 2 || dupF < 0 || market < 0 {
		t.Fatalf("fixture lookup failed: ori=%d thumbs=%v dupF=%d market=%d", oriID, thumbs, dupF, market)
	}

	// ori：两个 MD5A 缩略图 → 同一张原图（去重）。
	oris := out.selectAssociated(thumbs, []string{"origin"})
	if len(oris) != 1 || oris[0] != oriID {
		t.Fatalf("select(origin) on thumbs: got %v want [%d]", oris, oriID)
	}
	// ori：原文件保留自身。
	if got := out.selectAssociated([]int{oriID}, []string{"origin"}); len(got) != 1 || got[0] != oriID {
		t.Fatalf("select(origin) on ori: got %v want self", got)
	}
	// ori：无配对（marketface 无 md5）→ 无贡献。
	if got := out.selectAssociated([]int{market}, []string{"origin"}); len(got) != 0 {
		t.Fatalf("select(origin) on unpaired: got %v want empty", got)
	}

	// thumb：原文件 → 其全部缩略图（多尺寸）。
	got := out.selectAssociated([]int{oriID}, []string{"thumb"})
	if len(got) != 2 || got[0] != thumbs[0] || got[1] != thumbs[1] {
		t.Fatalf("select(thumb) on ori: got %v want %v", got, thumbs)
	}

	// dup：60KB 组任意一员 → 全组 3 份（含自身，跨账号）。
	dups := out.selectAssociated([]int{dupF}, []string{"dup"})
	if len(dups) != 3 {
		t.Fatalf("select(dup) on 60KB member: got %d want 3", len(dups))
	}
	found := false
	for _, d := range dups {
		if d == dupF {
			found = true
		}
	}
	if !found {
		t.Fatalf("select(dup) must keep the listed file itself, got %v want id %d", dups, dupF)
	}

	// 正交并集：select(ori, thumb) 在缩略图上 → {缩略图自身 ∪ 原图} = 整族。
	family := out.selectAssociated(thumbs, []string{"origin", "thumb"})
	if len(family) != 3 { // 2 thumbs + 1 ori（去重）
		t.Fatalf("select(ori,thumb) family: got %d want 3: %v", len(family), family)
	}
	for _, want := range append(append([]int{}, thumbs...), oriID) {
		found := false
		for _, d := range family {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("select(ori,thumb) missing id %d: %v", want, family)
		}
	}
	// 正交并集：select(thumb, dup) 在 60KB 组一员上 → 全组 3 份（组内
	// 都是缩略图且无配对 ori，thumb 维度贡献自身）。
	all := out.selectAssociated([]int{dupF}, []string{"thumb", "dup"})
	if len(all) != 3 {
		t.Fatalf("select(thumb,dup): got %d want 3", len(all))
	}

	// 经 Filter 走完整管道：md5F 的筛选 + select(dup) → 全组 3 条。
	backend := NewBackend("", nil)
	cap := newCaptureEmitter()
	backend.SetEmitter(cap)
	if err := backend.Scan(ScanOptions{Root: f.Root, MinAgeDays: 0}); err != nil {
		t.Fatal(err)
	}
	cap.waitFor(t, EvDone)
	stats, err := backend.GetStats(Filter{Expr: leaf("fileId", "eq", testutil.MD5F), Stages: []Stage{{Kind: "select", Kinds: []string{"dup"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Count != 3 {
		t.Fatalf("select(dup) via Filter: got %d want 3", stats.Count)
	}
	// 缩略图筛选 + select(origin)：只有 MD5A 有原图 → 1 条。
	oriStats, err := backend.GetStats(Filter{Expr: leaf("thumb", "eq", "true"), Stages: []Stage{{Kind: "select", Kinds: []string{"origin"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if oriStats.Count != 1 {
		t.Fatalf("select(origin) via Filter: got %d want 1 (only MD5A has an Ori)", oriStats.Count)
	}
	// 未知 select 类别：无贡献（前端解析器严格校验；这里是 API 误用兜底）。
	noop, err := backend.GetStats(Filter{Expr: leaf("thumb", "eq", "true"), Stages: []Stage{{Kind: "select", Kinds: []string{"nonsense"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if noop.Count != 0 {
		t.Fatalf("unknown select kind must contribute nothing: got %d want 0", noop.Count)
	}
	// 书写顺序语义：take(n) | select(dup)（先取 n 条再展开）与
	// select(dup) | take(n)（先展开再截断）结果不同。以 md5F 单条起手：
	// take(1) | select(dup) → 3（取到 md5F 后展开为内容组）；
	// select(dup) | take(1) → 1（展开后截断）。
	takeThenSelect, err := backend.GetStats(Filter{
		Expr:   leaf("fileId", "eq", testutil.MD5F),
		Stages: []Stage{{Kind: "take", N: 1}, {Kind: "select", Kinds: []string{"dup"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if takeThenSelect.Count != 3 {
		t.Fatalf("take(1)|select(dup): got %d want 3", takeThenSelect.Count)
	}
	selectThenTake, err := backend.GetStats(Filter{
		Expr:   leaf("fileId", "eq", testutil.MD5F),
		Stages: []Stage{{Kind: "select", Kinds: []string{"dup"}}, {Kind: "take", N: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selectThenTake.Count != 1 {
		t.Fatalf("select(dup)|take(1): got %d want 1", selectThenTake.Count)
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
