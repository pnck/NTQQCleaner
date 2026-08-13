package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"qqcleaner/internal/classify"
	"qqcleaner/internal/discovery"
	"qqcleaner/internal/qq"
	_ "qqcleaner/internal/qqimpl" // 注册 probe（QQ 平台×版本 dispatcher）
	"qqcleaner/internal/report"
	"qqcleaner/internal/rules"
)

// Engine is the stateless scan pipeline shared by CLI and GUI.
// discover → classify → score/tier → report (docs/04 §2).
type Engine struct {
	Cfg     rules.Config
	Emitter Emitter
	Now     func() time.Time // injectable clock; nil = time.Now
}

// Outcome is a finished scan held in memory for querying. Entries, Tiers
// and Reasons are index-aligned; ByAccount maps instance hashes to entry
// IDs; OriID/ThumbID map filename md5s to the ID of their Ori/Thumb entry
// for preview pairing (docs/07 §4.2). Now is the clock used for scoring
// and for age conditions (kept consistent per scan).
type Outcome struct {
	Root      string
	Accounts  []report.AccountReport
	Entries   []classify.FileEntry
	Tiers     []string
	Reasons   []string
	ByAccount map[string][]int
	MD5Index  rules.MD5Index
	OriID     map[string]int
	ThumbID   map[string]int
	Now       time.Time
	// K 是本次扫描分派到的 QQ 知识实现（清理/预览校验时复用）。
	K qq.Knowledge
}

// ScanAll scans every account under root (or just `accounts`), classifies,
// scores and aggregates. Progress events are emitted throttled (~100ms).
func (e *Engine) ScanAll(ctx context.Context, root string, accounts, onlyBizs []string, minAgeDays int, minSize int64) (*Outcome, error) {
	// 版本 dispatcher：从磁盘布局识别 QQ 平台×版本族，分派知识实现。
	// 不支持的布局（generic）fail-closed。
	k := qq.Detect(root)
	if !k.ScanCapable() {
		return nil, fmt.Errorf("unsupported QQ data layout (detected: %s); scanning disabled", k.Name())
	}
	accs, err := discovery.Discover(root)
	if err != nil {
		return nil, fmt.Errorf("discover %s: %w", root, err)
	}
	if len(accounts) > 0 {
		want := make(map[string]bool, len(accounts))
		for _, a := range accounts {
			want[a] = true
		}
		var kept []discovery.Account
		for _, a := range accs {
			if want[a.Hash] {
				kept = append(kept, a)
			}
		}
		accs = kept
	}
	if len(accs) == 0 {
		return nil, fmt.Errorf("no QQ account instance found under %s", root)
	}

	now := time.Now
	if e.Now != nil {
		now = e.Now
	}
	out := &Outcome{
		Root:      root,
		ByAccount: make(map[string][]int, len(accs)),
		MD5Index:  make(rules.MD5Index),
		OriID:     make(map[string]int),
		ThumbID:   make(map[string]int),
		Now:       now(),
		K:         k,
	}

	var (
		doneFiles uint64       // global file counter for the final event
		lastEmit  atomic.Int64 // unix millis of the last progress event
	)
	throttled := func(stage string, done, total uint64) {
		ms := time.Now().UnixMilli()
		prev := lastEmit.Load()
		if prev+100 > ms {
			return
		}
		if lastEmit.CompareAndSwap(prev, ms) && e.Emitter != nil {
			e.Emitter.Emit(EvProgress, Progress{Stage: stage, Done: done, Total: total})
		}
	}

	minAge := int64(minAgeDays) * 86400
	cutoff := now().Unix() - minAge

	for _, acc := range accs {
		if ctx.Err() != nil {
			break
		}
		// base = files accumulated from previous accounts (progress display).
		base := atomic.LoadUint64(&doneFiles)
		entries, err := classify.Scan(ctx, acc.NtData, classify.Options{
			K:        k,
			OnlyBizs: onlyBizs,
			SkipDirs: e.Cfg.SkipDirSet(),
			MinSize:  minSize,
			Progress: func(stage string, done, total uint64) {
				atomic.AddUint64(&doneFiles, 1)
				throttled(acc.Hash+"/"+filepath.Base(stage), base+done, total)
			},
		})
		if err != nil && ctx.Err() == nil {
			return nil, fmt.Errorf("scan %s: %w", acc.NtData, err)
		}

		acctIdx := rules.BuildMD5Index(entries)
		rep := report.AccountReport{
			Hash:        acc.Hash,
			QQNum:       acc.QQNum,
			NtData:      acc.NtData,
			LatestMonth: acc.LatestMonth,
			ByBiz:       make(map[string]report.TierTotals),
		}
		for _, f := range entries {
			if minAge > 0 && f.MTime > cutoff {
				continue // too fresh to even list (docs/04 §3 MinAgeDays)
			}
			id := len(out.Entries)
			out.Entries = append(out.Entries, f)
			t := now()
			score := rules.Score(k, f, acctIdx, e.Cfg, t)
			tier := rules.Tier(f, score, e.Cfg, t)
			out.Tiers = append(out.Tiers, tier)
			out.Reasons = append(out.Reasons, rules.Reason(f, tier, acctIdx))
			out.ByAccount[acc.Hash] = append(out.ByAccount[acc.Hash], id)
			if f.MD5 != "" {
				out.MD5Index[f.MD5] = acctIdx[f.MD5]
				if f.Sub == "Ori" {
					if _, ok := out.OriID[f.MD5]; !ok {
						out.OriID[f.MD5] = id
					}
				}
				if f.IsThumb {
					if _, ok := out.ThumbID[f.MD5]; !ok {
						out.ThumbID[f.MD5] = id
					}
				}
			}
			rep.TotalFiles++
			rep.TotalSize += f.Size
			rep.Totals.Add(tier, f.Size)
			b := rep.ByBiz[f.Biz]
			b.Add(tier, f.Size)
			rep.ByBiz[f.Biz] = b
		}
		out.Accounts = append(out.Accounts, rep)
	}
	if e.Emitter != nil {
		e.Emitter.Emit(EvProgress, Progress{Stage: "done", Done: doneFiles, Total: doneFiles})
	}
	return out, nil
}

// accountEntryIDs returns the entry IDs belonging to a hash (or all).
func (o *Outcome) accountEntryIDs(hash string) []int {
	if hash == "" {
		ids := make([]int, 0, len(o.Entries))
		for i := range o.Entries {
			ids = append(ids, i)
		}
		return ids
	}
	return o.ByAccount[hash]
}

// matchedIDs applies a Filter over the outcome, returning matching entry IDs.
func (o *Outcome) matchedIDs(f Filter) []int {
	ids := o.accountEntryIDs(f.Account)
	if f.Expr == nil {
		return ids
	}
	out := make([]int, 0, len(ids)/2+1)
	for _, id := range ids {
		if o.evalExpr(id, f.Expr) {
			out = append(out, id)
		}
	}
	return out
}

// applyStages applies the pipeline stages in order: order()（稳定多键排序）
// → drop → take. When the filter carries no order(), callers sort by the
// UI sort field first (QueryRows) or leave the natural order (aggregates).
func (o *Outcome) applyStages(ids []int, f Filter) []int {
	for _, ord := range f.Orders {
		sortIDs(o, ids, Sort{Field: ord.Field, Desc: ord.Desc})
	}
	if f.Offset > 0 {
		if f.Offset >= len(ids) {
			return nil
		}
		ids = ids[f.Offset:]
	}
	if f.Limit > 0 && f.Limit < len(ids) {
		ids = ids[:f.Limit]
	}
	return ids
}

// evalExpr evaluates the boolean tree against one entry.
func (o *Outcome) evalExpr(id int, e *Expr) bool {
	if e == nil {
		return true
	}
	if e.C != nil {
		return o.matchOne(id, *e.C)
	}
	if len(e.Or) > 0 {
		for _, sub := range e.Or {
			if o.evalExpr(id, sub) {
				return true
			}
		}
		return false
	}
	for _, sub := range e.And {
		if !o.evalExpr(id, sub) {
			return false
		}
	}
	return true
}

// matchOne evaluates a single condition against an entry. Unknown
// fields/operators fail closed (no match).
func (o *Outcome) matchOne(id int, c Condition) bool {
	e := o.Entries[id]
	ageDays := int64(o.Now.Sub(time.Unix(e.MTime, 0)).Hours() / 24)

	strVal := func() string {
		switch c.Field {
		case "biz":
			return e.Biz
		case "sub":
			return e.Sub
		case "category":
			return e.Category
		case "month":
			return e.Month
		case "md5":
			return e.MD5
		case "tier":
			return o.Tiers[id]
		case "reason":
			return o.Reasons[id]
		}
		return ""
	}
	numVal := func() int64 {
		switch c.Field {
		case "age":
			return ageDays
		case "size":
			return e.Size
		}
		return 0
	}
	boolVal := func() bool {
		switch c.Field {
		case "thumb":
			return e.IsThumb
		case "temp":
			return e.IsTemp
		}
		return false
	}

	switch c.Field {
	case "age", "size":
		n, err := strconv.ParseInt(strings.TrimSpace(c.Value), 10, 64)
		if err != nil {
			return false
		}
		v := numVal()
		switch c.Op {
		case "gt":
			return v > n
		case "gte":
			return v >= n
		case "lt":
			return v < n
		case "lte":
			return v <= n
		case "eq":
			return v == n
		case "ne":
			return v != n
		}
		return false
	case "thumb", "temp":
		want := strings.EqualFold(strings.TrimSpace(c.Value), "true")
		if c.Op == "ne" {
			return boolVal() != want
		}
		return boolVal() == want
	default: // string fields: biz/sub/category/month/md5/tier/reason
		v := strVal()
		switch c.Op {
		case "eq":
			return strings.EqualFold(v, strings.TrimSpace(c.Value))
		case "ne":
			return !strings.EqualFold(v, strings.TrimSpace(c.Value))
		case "in":
			for _, item := range strings.Split(c.Value, ",") {
				if strings.EqualFold(v, strings.TrimSpace(item)) {
					return true
				}
			}
			return false
		case "contains":
			return strings.Contains(strings.ToLower(v), strings.ToLower(strings.TrimSpace(c.Value)))
		case "gt", "gte", "lt", "lte":
			// 月份等可比字符串：YYYY-MM 字典序即时间序
			w := strings.TrimSpace(c.Value)
			switch c.Op {
			case "gt":
				return v > w
			case "gte":
				return v >= w
			case "lt":
				return v < w
			case "lte":
				return v <= w
			}
			return false
		}
		return false
	}
}
