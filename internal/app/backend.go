package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"qqcleaner/internal/classify"
	"qqcleaner/internal/clean"
	"qqcleaner/internal/discovery"
	"qqcleaner/internal/report"
	"qqcleaner/internal/rules"
)

// Backend is the stateful GUI-facing service (docs/04 §3, §4.5). All
// redline checks happen here in Go; the frontend is untrusted. Methods
// return errors instead of panicking; Scan/Clean are mutex-guarded against
// reentry (UI double-clicks).
type Backend struct {
	mu          sync.Mutex
	emitter     Emitter
	cfg         rules.Config
	cfgPath     string
	auditPath   string // "" = ~/.qq-cleaner/audit.log (injectable for tests)
	scanCtx     context.Context
	cancel      context.CancelFunc
	scanning    bool
	outcome     *Outcome
	lastErr     string
	previewBase string     // http://127.0.0.1:PORT（本地媒体预览服务）
	previewSrv  *http.Server
	previewOnce sync.Once
}

// NewBackend loads the config (or defaults when the file is absent),
// starts the loopback preview server and prepares an idle backend.
func NewBackend(cfgPath string, emitter Emitter) *Backend {
	cfg := rules.Default()
	if cfgPath != "" {
		if c, err := rules.Load(cfgPath); err == nil {
			cfg = c
		}
	}
	b := &Backend{cfg: cfg, cfgPath: cfgPath, emitter: emitter}
	b.ensurePreviewServer()
	return b
}

// ensurePreviewServer starts a loopback-only HTTP server for media
// previews. Full HTTP semantics (Range requests, streaming) are needed
// for video seeking — the Wails custom-protocol bridge does not forward
// them reliably. Paths stay whitelisted: every request goes through
// ResolvePreview. The port is randomized per session.
func (b *Backend) ensurePreviewServer() {
	b.previewOnce.Do(func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			b.lastErr = fmt.Sprintf("preview server: %v", err)
			return
		}
		b.previewBase = "http://" + ln.Addr().String()
		mux := http.NewServeMux()
		mux.HandleFunc("/preview/", b.PreviewHandler)
		srv := &http.Server{Handler: mux}
		b.previewSrv = srv
		go func() { _ = srv.Serve(ln) }()
	})
}

// SetEmitter swaps the event sink (used by tests and the GUI wiring).
func (b *Backend) SetEmitter(e Emitter) { b.mu.Lock(); b.emitter = e; b.mu.Unlock() }

func (b *Backend) emit(ev string, data any) {
	b.mu.Lock()
	e := b.emitter
	b.mu.Unlock()
	if e != nil {
		e.Emit(ev, data)
	}
}

// Emit implements Emitter so the Backend itself can be handed to the
// Engine as the event sink.
func (b *Backend) Emit(ev string, data any) { b.emit(ev, data) }

// ---- configuration ----

// GetConfig returns the current config (defaults applied).
func (b *Backend) GetConfig() rules.Config { b.mu.Lock(); defer b.mu.Unlock(); return b.cfg }

// SetConfig validates, persists and applies a config.
func (b *Backend) SetConfig(c rules.Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cfgPath != "" {
		if err := rules.Save(b.cfgPath, c); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}
	b.cfg = c
	return nil
}

// ---- discovery ----

// DiscoverRoots returns platform-default candidates plus any candidate that
// actually exists, existing ones first.
func (b *Backend) DiscoverRoots() []string {
	cands := discovery.RootCandidates()
	var existing, missing []string
	for _, c := range cands {
		if discovery.IsInstanceRoot(c) {
			existing = append(existing, c)
		} else {
			missing = append(missing, c)
		}
	}
	return append(existing, missing...)
}

// IsInstanceRoot validates a user-chosen root.
func (b *Backend) IsInstanceRoot(root string) bool { return discovery.IsInstanceRoot(root) }

// ---- scanning ----

// Scan launches an asynchronous dry-run scan. It returns an error only for
// invalid input; results arrive via scan:done / scan:error events.
func (b *Backend) Scan(opts ScanOptions) error {
	b.mu.Lock()
	if b.scanning {
		b.mu.Unlock()
		return fmt.Errorf("scan already running")
	}
	if opts.MinAgeDays <= 0 {
		opts.MinAgeDays = 3 // align with QQ's own 3-day baseline
	}
	cfg := b.cfg
	if opts.Aggressive {
		cfg.Aggressive = true
	}
	// GUI 模式：分类门控（clean_*）全部放开。价值判断已移交用户的
	// 筛选器（显式选择什么就清理什么）；结构性红线（nt_db/db 文件/
	// mmkv 等黑名单、路径穿越）在 clean 层照常强制。CLI 保持保守默认。
	cfg = cfgOpenGates(cfg)
	root := opts.Root
	if root == "" {
		cands := discovery.RootCandidates()
		for _, c := range cands {
			if discovery.IsInstanceRoot(c) {
				root = c
				break
			}
		}
	}
	if root == "" {
		b.mu.Unlock()
		return fmt.Errorf("no QQ data root found; pass one explicitly")
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.scanCtx, b.cancel = ctx, cancel
	b.scanning = true
	b.mu.Unlock()

	b.emit(EvState, map[string]bool{"scanning": true})
	go func() {
		defer func() {
			b.mu.Lock()
			b.scanning = false
			b.cancel = nil
			b.mu.Unlock()
			b.emit(EvState, map[string]bool{"scanning": false})
		}()
		eng := &Engine{Cfg: cfg, Emitter: b}
		out, err := eng.ScanAll(ctx, root, nil, opts.OnlyBizs, opts.MinAgeDays, opts.MinSize)
		b.mu.Lock()
		b.lastErr = ""
		if err != nil {
			if ctx.Err() != nil {
				b.lastErr = "scan stopped"
			} else {
				b.lastErr = err.Error()
			}
		}
		if out != nil {
			b.outcome = out
		}
		reports := b.accountsLocked()
		msg := map[string]any{"root": root, "accounts": reports, "error": b.lastErr}
		b.mu.Unlock()
		if err != nil {
			b.emit(EvError, msg)
		} else {
			b.emit(EvDone, msg)
		}
	}()
	return nil
}

// Stop cancels a running scan (docs/07 §3: 取消扫描，保留已扫部分).
func (b *Backend) Stop() {
	b.mu.Lock()
	cancel := b.cancel
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ScanState reports whether a scan is running (UI re-attach).
func (b *Backend) ScanState() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return map[string]any{
		"scanning": b.scanning,
		"root":     b.outcomeRoot(),
		"accounts": b.accountsLocked(),
	}
}

func (b *Backend) outcomeRoot() string {
	if b.outcome != nil {
		return b.outcome.Root
	}
	return ""
}

func (b *Backend) accountsLocked() []report.AccountReport {
	if b.outcome == nil {
		return nil
	}
	return b.outcome.Accounts
}

// ---- querying ----

// QueryRows returns one page of FileRows (docs/07 §6). IDs are stable
// across queries within one scan; preview URLs resolve via ResolvePreview.
func (b *Backend) QueryRows(q PageQuery) (PageResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.outcome
	if out == nil {
		return PageResult{}, fmt.Errorf("no scan results yet")
	}
	ids := out.matchedIDs(q.Filter)
	if q.Sort.Field != "" {
		sortIDs(out, ids, q.Sort)
	}
	// take/drop 管道在排序后应用（take = 排序后前 n 条）
	ids = out.applyStages(ids, q.Filter)
	total := len(ids)
	if q.PageSize <= 0 {
		q.PageSize = 200
	}
	if q.PageSize > 500 {
		q.PageSize = 500
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	start := (q.Page - 1) * q.PageSize
	if start > total {
		start = total
	}
	end := start + q.PageSize
	if end > total {
		end = total
	}
	rows := make([]report.FileRow, 0, end-start)
	for _, id := range ids[start:end] {
		rows = append(rows, b.fileRowLocked(out, id))
	}
	return PageResult{Total: total, Page: q.Page, Rows: rows}, nil
}

// fileRowLocked builds a UI row for one entry ID.
func (b *Backend) fileRowLocked(out *Outcome, id int) report.FileRow {
	e := out.Entries[id]
	row := report.FileRow{
		ID:     id,
		MD5:    e.MD5,
		Biz:    e.Biz,
		Sub:    e.Sub,
		Month:  e.Month,
		Size:   e.Size,
		MTime:  e.MTime,
		Ext:    e.Ext,
		Tier:   out.Tiers[id],
		Reason: out.Reasons[id],
	}
	row.ThumbURL = b.previewURL(id)
	if e.IsThumb {
		if oriID, ok := out.OriID[e.MD5]; ok {
			row.OriURL = b.previewURL(oriID)
		}
	} else if e.Sub == "Ori" {
		if thumbID, ok := out.ThumbID[e.MD5]; ok {
			row.ThumbURL = b.previewURL(thumbID)
		}
	}
	return row
}

// sortIDs sorts matched IDs by field (docs/07 §5 中栏).
func sortIDs(out *Outcome, ids []int, s Sort) {
	order := map[string]int{"safe": 0, "suggest": 1, "caution": 2, "keep": 3}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := out.Entries[ids[i]], out.Entries[ids[j]]
		var less bool
		switch s.Field {
		case "size":
			less = a.Size < b.Size
		case "mtime":
			less = a.MTime < b.MTime
		case "tier":
			less = order[out.Tiers[ids[i]]] < order[out.Tiers[ids[j]]]
		case "month":
			less = a.Month < b.Month
		default: // md5
			less = a.MD5 < b.MD5
		}
		if s.Desc {
			return !less
		}
		return less
	})
}

// GetTotals aggregates tiers over a filter (docs/07 §5 底栏).
func (b *Backend) GetTotals(f Filter) (report.TierTotals, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.outcome == nil {
		return report.TierTotals{}, fmt.Errorf("no scan results yet")
	}
	var t report.TierTotals
	for _, id := range b.outcome.applyStages(b.outcome.matchedIDs(f), f) {
		t.Add(b.outcome.Tiers[id], b.outcome.Entries[id].Size)
	}
	return t, nil
}

// GetStats returns the file count and total size of a filter
// (bottom bar). The tier totals are intentionally not part of the
// primary UI anymore: the safety tiers are a reference field, not the
// browsing axis (user-editable filters are).
func (b *Backend) GetStats(f Filter) (Stats, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.outcome == nil {
		return Stats{}, fmt.Errorf("no scan results yet")
	}
	var s Stats
	for _, id := range b.outcome.applyStages(b.outcome.matchedIDs(f), f) {
		s.Count++
		s.Size += b.outcome.Entries[id].Size
	}
	return s, nil
}

// GetIDs returns every entry ID matching a filter (for "select all in
// filter"). IDs are index-stable within one scan.
func (b *Backend) GetIDs(f Filter) ([]int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.outcome == nil {
		return nil, fmt.Errorf("no scan results yet")
	}
	return b.outcome.applyStages(b.outcome.matchedIDs(f), f), nil
}

// GetDupes finds md5 groups with ≥2 copies in the whole index and, for
// each, which copies inside the current filter can be removed while
// keeping exactly one global copy (prefer the Ori original, then the
// newest mtime). Groups are ordered by removable bytes desc, capped at
// 500 for the UI.
func (b *Backend) GetDupes(f Filter) ([]DupGroup, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.outcome
	if out == nil {
		return nil, fmt.Errorf("no scan results yet")
	}
	// md5 → all ids（全索引，非筛选）
	byMD5 := make(map[string][]int)
	for id, e := range out.Entries {
		if e.MD5 != "" {
			byMD5[e.MD5] = append(byMD5[e.MD5], id)
		}
	}
	inFilter := make(map[int]bool)
	for _, id := range out.applyStages(out.matchedIDs(f), f) {
		inFilter[id] = true
	}
	var groups []DupGroup
	for md5, ids := range byMD5 {
		if len(ids) < 2 {
			continue
		}
		// 保留份：Ori 优先，其次 mtime 最新
		keep := ids[0]
		for _, id := range ids[1:] {
			e, k := out.Entries[id], out.Entries[keep]
			if (e.Sub == "Ori") != (k.Sub == "Ori") {
				if e.Sub == "Ori" {
					keep = id
				}
				continue
			}
			if e.MTime > k.MTime {
				keep = id
			}
		}
		g := DupGroup{
			MD5:       md5,
			Count:     len(ids),
			KeepID:    keep,
			KeepLabel: dupLabel(out.Entries[keep]),
			KeepMTime: out.Entries[keep].MTime,
		}
		for _, id := range ids {
			if id == keep {
				g.TotalBytes += out.Entries[id].Size
				continue
			}
			g.TotalBytes += out.Entries[id].Size
			if inFilter[id] {
				g.DupIDs = append(g.DupIDs, id)
				g.DupBytes += out.Entries[id].Size
			}
		}
		if len(g.DupIDs) > 0 {
			groups = append(groups, g)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].DupBytes > groups[j].DupBytes })
	if len(groups) > 500 {
		groups = groups[:500]
	}
	return groups, nil
}

func dupLabel(e classify.FileEntry) string {
	return e.Month + "/" + e.Sub + "/" + filepath.Base(e.Path)
}

// GetGroups aggregates the left tree: by="biz" or by="month"
// (docs/07 §5 左栏).
func (b *Backend) GetGroups(f Filter, by string) ([]report.GroupStat, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.outcome == nil {
		return nil, fmt.Errorf("no scan results yet")
	}
	groups := make(map[string]*report.GroupStat)
	for _, id := range b.outcome.matchedIDs(f) {
		e := b.outcome.Entries[id]
		key := e.Biz
		if by == "month" {
			key = e.Month
			if key == "" {
				continue
			}
		}
		g := groups[key]
		if g == nil {
			g = &report.GroupStat{Key: key}
			groups[key] = g
		}
		g.Count++
		g.Size += e.Size
		g.Totals.Add(b.outcome.Tiers[id], e.Size)
	}
	out := make([]report.GroupStat, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out, nil
}

// ---- preview ----

// previewURL builds the media URL for one entry ID (loopback HTTP server).
func (b *Backend) previewURL(id int) string {
	if b.previewBase == "" {
		return "/preview/" + strconv.Itoa(id) // fallback: asset-server route
	}
	return b.previewBase + "/preview/" + strconv.Itoa(id)
}

// ResolvePreview maps a preview ID to the absolute file path, re-checking
// the whitelist (docs/07 §4.1: 白名单校验在 handler 内).
func (b *Backend) ResolvePreview(id int) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.outcome == nil || id < 0 || id >= len(b.outcome.Entries) {
		return "", fmt.Errorf("bad preview id")
	}
	e := b.outcome.Entries[id]
	roots := b.outcomeRootsLocked()
	if err := clean.VerifyPath(e.Path, roots, b.cfg); err != nil {
		return "", err
	}
	if st, err := os.Stat(e.Path); err != nil || st.IsDir() {
		return "", fmt.Errorf("preview file unavailable")
	}
	return e.Path, nil
}

func (b *Backend) outcomeRootsLocked() []string {
	if b.outcome == nil {
		return nil
	}
	seen := make(map[string]bool)
	var roots []string
	for _, a := range b.outcome.Accounts {
		if !seen[a.NtData] {
			seen[a.NtData] = true
			roots = append(roots, a.NtData)
		}
	}
	return roots
}

// Reveal opens the platform file manager with the file selected
// (docs/07 §4.4: 仅展示不操作).
func (b *Backend) Reveal(id int) error {
	p, err := b.ResolvePreview(id)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", p)
	case "windows":
		cmd = exec.Command("explorer", "/select,", p)
	default:
		return fmt.Errorf("reveal not supported on %s", runtime.GOOS)
	}
	return cmd.Start()
}

// PreviewHandler serves /preview/{id} as the file's content (Wails
// AssetsHandler). Range requests (video seeking) are handled by ServeFile.
func (b *Backend) PreviewHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/preview/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := b.ResolvePreview(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}

// ---- cleanup ----

// cfgOpenGates enables every category gate so the GUI whitelist check only
// enforces structure + blacklist (see Scan; the user's filter is the
// selection mechanism).
func cfgOpenGates(cfg rules.Config) rules.Config {
	cfg.CleanTemp = true
	cfg.CleanThumb = true
	cfg.CleanOri = true
	cfg.CleanBaseEmoji = true
	cfg.CleanMarketface = true
	cfg.CleanPersonalEmoji = true
	cfg.CleanFile = true
	cfg.CleanLog = true
	cfg.CleanAvatar = true
	return cfg
}

// Clean executes a cleanup for the selected rows. The frontend dialog's
// confirm becomes req.Confirmed; every file is re-verified inside
// clean.Run regardless (docs/06: UI 不可信).
func (b *Backend) Clean(req CleanRequest) (CleanResult, error) {
	b.mu.Lock()
	if b.scanning {
		b.mu.Unlock()
		return CleanResult{}, fmt.Errorf("scan running; stop it first")
	}
	out := b.outcome
	cfg := b.cfg
	if out == nil {
		b.mu.Unlock()
		return CleanResult{}, fmt.Errorf("no scan results yet")
	}
	var files []classify.FileEntry
	for _, id := range req.IDs {
		if id < 0 || id >= len(out.Entries) {
			continue
		}
		tier := out.Tiers[id]
		// Only cleanable tiers can be selected; the clean layer re-verifies.
		switch tier {
		case rules.TierSafe, rules.TierSuggest, rules.TierCaution:
			files = append(files, out.Entries[id])
		}
	}
	roots := b.outcomeRootsLocked()
	auditLog := b.auditPath
	if auditLog == "" {
		auditLog = filepath.Join(configDir(), "audit.log")
	}
	b.mu.Unlock()

	if len(files) == 0 {
		return CleanResult{}, fmt.Errorf("nothing selected to clean")
	}
	res, err := clean.Run(context.Background(), clean.Request{
		Files:        files,
		AllowedRoots: roots,
		BackupDir:    req.BackupDir,
		AuditLog:     auditLog,
		Force:        req.Force,
		Confirmed:    req.Confirmed,
		Config:       cfgOpenGates(cfg), // 结构红线不变，分类门控随 GUI 放开
	})
	if err != nil {
		return CleanResult{}, err
	}
	// The index is now stale (files are gone); force a rescan before any
	// further queries so stale IDs can never be cleaned twice.
	b.mu.Lock()
	b.outcome = nil
	b.mu.Unlock()
	return CleanResult{
		Processed:  res.Processed,
		Moved:      res.Moved,
		Deleted:    res.Deleted,
		Skipped:    res.Skipped,
		Failed:     res.Failed,
		BytesFreed: res.BytesFreed,
		Errors:     res.Errors,
	}, nil
}

// ConfigDir returns the tool's config directory (~/.qq-cleaner).
func ConfigDir() string { return configDir() }

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".qq-cleaner"
	}
	return filepath.Join(home, ".qq-cleaner")
}
