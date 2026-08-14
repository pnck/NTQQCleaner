package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"qqcleaner/internal/classify"
	"qqcleaner/internal/clean"
	"qqcleaner/internal/discovery"
	"qqcleaner/internal/platform"
	"qqcleaner/internal/qq"
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
	previewBase string // http://127.0.0.1:PORT（本地媒体预览服务）
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
	cands := qq.RootCandidates()
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
		opts.MinAgeDays = rules.DefaultMinAgeDays // 对齐 QQ 官方 3 天基线
	}
	cfg := b.cfg
	// GUI 模式：普通类别门控全部放开（选择权在用户的筛选器），
	// 高级 opt-in 类别（传输残留/日志/头像）保持用户在设置里的选择
	// ——默认关闭的类别不进索引。结构性红线在 clean 层照常强制。
	cfg = cfgOpenGates(cfg)
	root := opts.Root
	if root == "" {
		cands := qq.RootCandidates()
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
		out, err := eng.ScanAll(ctx, root, nil, opts.OnlyBizs, opts.MinAgeDays, opts.MinSize, rules.GatesOf(cfg))
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
	// 管道在 UI 排序后按书写顺序应用
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
		ID:              id,
		MD5:             e.MD5,
		Biz:             e.Biz,
		Sub:             e.Sub,
		Month:           e.Month,
		Size:            e.Size,
		MTime:           e.MTime,
		Ext:             e.Ext,
		Reason:          out.Reasons[id],
		ContentHash:     e.ContentHash,
		ContentDupCount: len(out.ContentIndex[e.ContentHash]),
	}
	row.ThumbURL = b.previewURL(id)
	if e.IsThumb {
		// 缩略图行：OriURL 指向同 md5 的原文件，OriExt 取其扩展名
		// （缩略图行自身 ext 是缩略图扩展名，不能用于分派原文件的播放器）
		if oriID, ok := out.OriID[e.MD5]; ok {
			row.OriURL = b.previewURL(oriID)
			row.OriExt = out.Entries[oriID].Ext
		}
	} else if e.Sub == "Ori" {
		// 原文件行：OriURL 就是它自己（视频/动图/原图的直接预览入口）
		row.OriURL = b.previewURL(id)
		row.OriExt = e.Ext
		if thumbID, ok := out.ThumbID[e.MD5]; ok {
			row.ThumbURL = b.previewURL(thumbID)
		}
	}
	return row
}

// sortIDs sorts matched IDs by field (docs/07 §5 中栏).
func sortIDs(out *Outcome, ids []int, s Sort) {
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := out.Entries[ids[i]], out.Entries[ids[j]]
		var less bool
		switch s.Field {
		case "size":
			less = a.Size < b.Size
		case "mtime":
			less = a.MTime < b.MTime
		case "month":
			less = a.Month < b.Month
		default: // fileId
			less = a.MD5 < b.MD5
		}
		if s.Desc {
			return !less
		}
		return less
	})
}

// GetStats returns the file count and total size of a filter
// (bottom bar).
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

// GetDupes finds byte-identical groups (SHA-256 content hash) across the
// whole index and, for each, which copies inside the current filter can be
// removed while keeping exactly one global copy (prefer the Ori original,
// then the newest mtime — the keeper may live outside the filter).
//
// 面板的可去重项 = 组内副本 ∩ 当前筛选（去重建议只是另一种筛选器，
// docs/07 §3b）：筛选之外的副本只影响「保留谁」，不出现在可去重项里。
// Groups are ordered by removable bytes desc, capped at 500 for the UI.
// 分组依据是二次扫描的内容哈希而非文件名 md5：QQ 只按目录去重，同一
// 内容在不同目录/月份可能以不同名字各存一份；文件名 md5 配对（Ori/
// Thumb 同名）不是字节级重复，不再进入去重组。
func (b *Backend) GetDupes(f Filter) ([]DupGroup, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.outcome
	if out == nil {
		return nil, fmt.Errorf("no scan results yet")
	}
	inFilter := make(map[int]bool)
	for _, id := range out.applyStages(out.matchedIDs(f), f) {
		inFilter[id] = true
	}
	var groups []DupGroup
	for hash, ids := range out.ContentIndex {
		if len(ids) < 2 {
			continue
		}
		keep := out.pickKeeper(ids)
		g := DupGroup{
			Hash:         hash,
			Count:        len(ids),
			KeepID:       keep,
			KeepLabel:    dupLabel(out.Entries[keep]),
			KeepMTime:    out.Entries[keep].MTime,
			KeepInFilter: inFilter[keep],
		}
		for _, id := range ids {
			g.TotalBytes += out.Entries[id].Size
			if id == keep {
				continue
			}
			if inFilter[id] {
				g.DupIDs = append(g.DupIDs, id)
				g.DupBytes += out.Entries[id].Size
			}
		}
		if len(g.DupIDs) > 0 {
			groups = append(groups, g)
		}
	}
	return sortDupGroups(groups), nil
}

// pickKeeper 在内容组内选保留份：Ori 优先，其次 mtime 最新。
func (o *Outcome) pickKeeper(ids []int) int {
	keep := ids[0]
	for _, id := range ids[1:] {
		e, k := o.Entries[id], o.Entries[keep]
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
	return keep
}

// sortDupGroups 按可释放字节降序，上限 500 组（UI 性能预算）。
func sortDupGroups(groups []DupGroup) []DupGroup {
	sort.Slice(groups, func(i, j int) bool { return groups[i].DupBytes > groups[j].DupBytes })
	if len(groups) > 500 {
		groups = groups[:500]
	}
	return groups
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
	}
	out := make([]report.GroupStat, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	if by == "month" {
		// 月份按时间倒序（YYYY-MM 字典序即时间序，新→旧），与工具栏
		// 「月份」排序默认方向一致；业务类型仍按可清大小倒序。
		sort.Slice(out, func(i, j int) bool { return out[i].Key > out[j].Key })
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	}
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

// ResolvePreview maps a preview ID to the absolute file path. The path
// comes from the scan index (server-side, not frontend input); it is
// re-checked structurally (roots/traversal/blacklist, docs/07 §4.1) but
// NOT against cleanability gates — report-only（门控关闭类别）条目必须
// remain viewable/revealable.
func (b *Backend) ResolvePreview(id int) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.outcome == nil || id < 0 || id >= len(b.outcome.Entries) {
		return "", fmt.Errorf("bad preview id")
	}
	e := b.outcome.Entries[id]
	roots := b.outcomeRootsLocked()
	if err := clean.VerifyStructural(b.outcome.K, e.Path, roots); err != nil {
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
// (docs/07 §4.4: 仅展示不操作)。实现委托 platform 适配层。
func (b *Backend) Reveal(id int) error {
	p, err := b.ResolvePreview(id)
	if err != nil {
		return err
	}
	return platform.Current().Reveal(p)
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

// errQQRunningSentinel 是 Backend.Clean 的 QQ 运行预检哨兵错误；
// 前端识别后弹二次确认，确认后带 ignoreRunning 重试。
var errQQRunningSentinel = fmt.Errorf("qq-running")

// cfgOpenGates enables the ordinary category gates so the GUI whitelist
// check only enforces structure + blacklist for them (the user's filter is
// the selection mechanism). The advanced opt-in gates (CleanLog /
// CleanDatalineTmp / CleanAvatar) keep the user's settings choice — off by
// default, so those categories are neither scanned nor cleanable until
// enabled in 设置 → 高级.
func cfgOpenGates(cfg rules.Config) rules.Config {
	cfg.CleanTemp = true
	cfg.CleanThumb = true
	cfg.CleanOri = true
	cfg.CleanBaseEmoji = true
	cfg.CleanMarketface = true
	cfg.CleanPersonalEmoji = true
	cfg.CleanFile = true
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
		// 选择权在用户的勾选；每个文件在 clean.Run 内被重新校验
		// （白名单/黑名单/路径穿越，docs/06 §5b）。
		files = append(files, out.Entries[id])
	}
	roots := b.outcomeRootsLocked()
	auditLog := b.auditPath
	if auditLog == "" {
		// 每次清理生成一份带时间戳的临时审计报告（系统 tmp），清理后
		// 打开供用户查看/另存——不轮转、不积累。
		auditLog = filepath.Join(configDir(), "audit-"+time.Now().Format("20060102-150405")+".jsonl")
	}
	b.mu.Unlock()

	if len(files) == 0 {
		return CleanResult{}, fmt.Errorf("nothing selected to clean")
	}
	// 预检：QQ 运行中 → 返回哨兵错误，前端据以下发二次确认后带
	// ignoreRunning=true 重试（clean 层另有自身检查，双保险）。
	if !req.IgnoreRunning && clean.QQRunning() {
		return CleanResult{}, errQQRunningSentinel
	}
	res, err := clean.Run(context.Background(), clean.Request{
		Files:         files,
		AllowedRoots:  roots,
		BackupDir:     req.BackupDir,
		AuditLog:      auditLog,
		Force:         req.Force,
		Confirmed:     req.Confirmed,
		IgnoreRunning: req.IgnoreRunning,
		K:             out.K,             // 本扫描分派的知识实现（白名单/打分）
		Config:        cfgOpenGates(cfg), // 结构红线不变，分类门控随 GUI 放开
	})
	if err != nil {
		return CleanResult{}, err
	}
	// The index is now stale (files are gone); force a rescan before any
	// further queries so stale IDs can never be cleaned twice.
	b.mu.Lock()
	b.outcome = nil
	b.mu.Unlock()
	items := make([]CleanItem, 0, len(res.Items))
	for _, it := range res.Items {
		items = append(items, CleanItem{
			Path:       it.Path,
			Action:     it.Action,
			BackupPath: it.BackupPath,
			Reason:     it.Reason,
			Size:       it.Size,
		})
	}
	// 审计报告已生成：打开供用户查看/另存（失败不影响清理结果）。
	if err := platform.Current().OpenFile(auditLog); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("audit report %s: %v", auditLog, err))
	}
	return CleanResult{
		Processed:  res.Processed,
		Moved:      res.Moved,
		Deleted:    res.Deleted,
		Skipped:    res.Skipped,
		Failed:     res.Failed,
		BytesFreed: res.BytesFreed,
		Items:      items,
		AuditPath:  auditLog,
		Errors:     res.Errors,
	}, nil
}

// ConfigDir returns the tool's own data directory — 系统 tmp 下的
// ntqq-cleaner/（非 $HOME 隐藏目录）：工具写入的全部文件都是可弃的，
// 由 OS 清理；config 与审计报告在本次开机内跨启动复用。
func ConfigDir() string { return configDir() }

func configDir() string {
	d := filepath.Join(os.TempDir(), "ntqq-cleaner")
	_ = os.MkdirAll(d, 0o755)
	return d
}
