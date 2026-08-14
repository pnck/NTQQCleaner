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
// discover → classify → 内容哈希二次扫描 → reason/关联索引 → report
// (docs/04 §2).
type Engine struct {
	Cfg     rules.Config
	Emitter Emitter
	Now     func() time.Time // injectable clock; nil = time.Now
}

// Outcome is a finished scan held in memory for querying. Entries and
// Reasons are index-aligned; ByAccount maps instance hashes to entry IDs;
// OriID/ThumbID map filename md5s to the ID of their Ori/Thumb entry for
// preview pairing (docs/07 §4.2); ContentIndex maps SHA-256 content hashes
// to entry IDs (byte-identical copies, 二次扫描填充). Now is the clock
// used for age conditions (kept consistent per scan).
type Outcome struct {
	Root         string
	Accounts     []report.AccountReport
	Entries      []classify.FileEntry
	Reasons      []string
	ByAccount    map[string][]int
	OriID        map[string]int
	ThumbID      map[string]int   // 预览配对：每个 md5 取第一个缩略图
	ThumbIDs     map[string][]int // select(thumb)：每个 md5 的全部缩略图
	ContentIndex map[string][]int
	Now          time.Time
	// K 是本次扫描分派到的 QQ 知识实现（清理/预览校验时复用）。
	K qq.Knowledge
}

// ScanAll scans every account under root (or just `accounts`), classifies
// and aggregates. Progress events are emitted throttled (~100ms).
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
		Root:         root,
		ByAccount:    make(map[string][]int, len(accs)),
		OriID:        make(map[string]int),
		ThumbID:      make(map[string]int),
		ThumbIDs:     make(map[string][]int),
		ContentIndex: make(map[string][]int),
		Now:          now(),
		K:            k,
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

	// 第一遍：逐账号 classify（太新的文件连列表都不进，docs/04 §3）。
	type acctScan struct {
		acc  discovery.Account
		kept []classify.FileEntry
	}
	var scans []acctScan
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
		var kept []classify.FileEntry
		for _, f := range entries {
			if minAge > 0 && f.MTime > cutoff {
				continue
			}
			kept = append(kept, f)
		}
		scans = append(scans, acctScan{acc: acc, kept: kept})
	}
	// 第二遍（全局）：为「与其它文件字节数完全相同」的文件计算 SHA-256
	// 内容哈希——去重/「重复出现」的字节级依据。跨账号进行（同一内容
	// 可能存在不同账号/不同目录下）。只读不写；可取消。
	var all []*classify.FileEntry
	for _, s := range scans {
		for i := range s.kept {
			all = append(all, &s.kept[i])
		}
	}
	if err := classify.HashDuplicates(ctx, all, func(done, total uint64) {
		throttled("hash", done, total)
	}); err != nil && ctx.Err() == nil {
		return nil, fmt.Errorf("content hash pass: %w", err)
	}

	// 组装：entries/reasons 之外的索引（ByAccount/OriID/ThumbID/ContentIndex）。
	for _, s := range scans {
		rep := report.AccountReport{
			Hash:        s.acc.Hash,
			QQNum:       s.acc.QQNum,
			NtData:      s.acc.NtData,
			LatestMonth: s.acc.LatestMonth,
		}
		for _, f := range s.kept {
			id := len(out.Entries)
			out.Entries = append(out.Entries, f)
			out.ByAccount[s.acc.Hash] = append(out.ByAccount[s.acc.Hash], id)
			if f.MD5 != "" {
				if f.Sub == "Ori" {
					if _, ok := out.OriID[f.MD5]; !ok {
						out.OriID[f.MD5] = id
					}
				}
				if f.IsThumb {
					if _, ok := out.ThumbID[f.MD5]; !ok {
						out.ThumbID[f.MD5] = id
					}
					out.ThumbIDs[f.MD5] = append(out.ThumbIDs[f.MD5], id)
				}
			}
			if f.ContentHash != "" {
				out.ContentIndex[f.ContentHash] = append(out.ContentIndex[f.ContentHash], id)
				rep.HashedFiles++
			}
			rep.TotalFiles++
			rep.TotalSize += f.Size
		}
		out.Accounts = append(out.Accounts, rep)
	}
	// 关联标签统一计算：内容重复计数来自跨账号的 ContentIndex；
	// 配对（原图仍在/有缩略图）来自文件名 md5 的 OriID/ThumbID。
	out.Reasons = make([]string, len(out.Entries))
	for i, ent := range out.Entries {
		_, hasOri := out.OriID[ent.MD5]
		_, hasThumb := out.ThumbID[ent.MD5]
		cnt := 0
		if ent.ContentHash != "" {
			cnt = len(out.ContentIndex[ent.ContentHash])
		}
		out.Reasons[i] = rules.Reason(ent, hasOri, hasThumb, cnt)
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

// applyStages applies the pipeline in **written order**（函数式组合，
// docs/04 §3）：每个 stage 作用于前一 stage 的输出。take(10) | select(dup)
// 先取 10 条再展开，与 select(dup) | take(10) 语义不同。未知 stage 种类
// 无效果（fail closed）。QueryRows 会在管道前按 UI 排序字段预排序
// （无 order() 时即最终顺序；有 order() 时在其位置重排）。
func (o *Outcome) applyStages(ids []int, f Filter) []int {
	for _, s := range f.Stages {
		switch s.Kind {
		case "select":
			ids = o.selectAssociated(ids, s.Kinds)
		case "order":
			sortIDs(o, ids, Sort{Field: s.Field, Desc: s.Desc})
		case "drop":
			if s.N <= 0 {
				continue
			}
			if s.N >= len(ids) {
				return nil
			}
			ids = ids[s.N:]
		case "take":
			if s.N > 0 && s.N < len(ids) {
				ids = ids[:s.N]
			}
		}
	}
	return ids
}

// selectAssociated 把结果集替换为其中文件关联的另一组文件（docs/04 §3
// 管道 select()）。kinds 是正交的展开维度，取**并集**（去重）：
//   - ori   ：缩略图 → 其原文件；原文件保留自身；无配对（无 md5/无 Ori）
//     的文件对本维度无贡献
//   - thumb ：原文件 → 其全部缩略图（多尺寸）；缩略图保留自身
//   - dup   ：展开为内容哈希组（字节级相同的全部文件，含列表内的自身）；
//     无哈希（大小唯一）的文件对本维度无贡献
//
// 未知类别视为无贡献（前端解析器已严格校验，这里防 API 误用）。
//
// 关联依据：ori/thumb 走文件名 md5 配对（同名关系）；dup 走二次扫描的
// SHA-256 内容组（同名 ≠ 同内容，同内容可能不同名）。
func (o *Outcome) selectAssociated(ids []int, kinds []string) []int {
	seen := make(map[int]bool, len(ids)*2)
	var out []int
	add := func(id int) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range ids {
		e := o.Entries[id]
		for _, kind := range kinds {
			switch kind {
			case "ori":
				if strings.EqualFold(e.Sub, "Ori") {
					add(id)
					continue
				}
				if ori, ok := o.OriID[e.MD5]; ok {
					add(ori)
				}
			case "thumb":
				if e.IsThumb {
					add(id)
					continue
				}
				for _, t := range o.ThumbIDs[e.MD5] {
					add(t)
				}
			case "dup":
				if e.ContentHash == "" {
					continue
				}
				for _, d := range o.ContentIndex[e.ContentHash] {
					add(d)
				}
			}
		}
	}
	return out
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
		case "md5":
			return e.MD5
		case "contentHash":
			return e.ContentHash
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
		v := numVal()
		if c.Op == "in" {
			// 列表形式：逐项解析，跳过不可解析项
			for _, item := range strings.Split(c.Value, ",") {
				n2, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
				if err == nil && v == n2 {
					return true
				}
			}
			return false
		}
		n, err := strconv.ParseInt(strings.TrimSpace(c.Value), 10, 64)
		if err != nil {
			return false
		}
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
	case "month":
		// 月份操作数按可计算时间比较：YYYY-MM 解析为当月起始时间戳
		// （实现细节不对用户暴露）。字符串序在跨年/缺位写法下不可靠，
		// 解析失败一律不匹配（fail closed）。contains 保留在规范字符串
		// 上做子串匹配（month ~ 2025 = 全年），与顺序无关。
		mv, ok := monthVal(e.Month)
		if !ok {
			return false
		}
		switch c.Op {
		case "eq":
			w, ok := monthVal(c.Value)
			return ok && mv == w
		case "ne":
			w, ok := monthVal(c.Value)
			return !ok || mv != w
		case "gt", "gte", "lt", "lte":
			w, ok := monthVal(c.Value)
			if !ok {
				return false
			}
			switch c.Op {
			case "gt":
				return mv > w
			case "gte":
				return mv >= w
			case "lt":
				return mv < w
			case "lte":
				return mv <= w
			}
			return false
		case "in":
			for _, item := range strings.Split(c.Value, ",") {
				w, ok := monthVal(item)
				if ok && mv == w {
					return true
				}
			}
			return false
		case "contains":
			return strings.Contains(e.Month, strings.TrimSpace(c.Value))
		}
		return false
	default: // string fields: biz/sub/category/md5/contentHash/reason
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
			// 可比字符串（biz/sub/category 等）：字典序
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

// monthVal 把 "YYYY-MM" 解析为当月起始时间戳，供月份操作数的可计算比较
// （不按字符串序）。解析失败返回 ok=false —— 调用方 fail closed。
func monthVal(s string) (int64, bool) {
	t, err := time.Parse("2006-01", strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return t.Unix(), true
}
