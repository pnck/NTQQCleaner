// Package app is the shared scan pipeline (Engine) and the GUI-facing
// backend bound to the Wails frontend. It deliberately does NOT import
// Wails: events go through the Emitter interface so the whole layer is
// headless-testable, and the CLI uses the very same Engine (docs/04 §8).
package app

import "qqcleaner/internal/report"

// Emitter delivers progress/result events. The GUI wires it to
// runtime.EventsEmit; the CLI passes nil.
type Emitter interface {
	Emit(event string, data any)
}

// Event names (frontend subscribes via wails Events.On).
const (
	EvProgress = "scan:progress"
	EvDone     = "scan:done"
	EvError    = "scan:error"
	EvState    = "scan:state"
)

// Progress is one throttled scan progress event.
type Progress struct {
	Stage string `json:"stage"` // e.g. account hash + biz dir
	Done  uint64 `json:"done"`
	Total uint64 `json:"total"`
}

// ScanOptions mirrors docs/04 §3.
type ScanOptions struct {
	Root       string   `json:"root"`    // "" = auto-detect
	Account    string   `json:"account"` // "" = all accounts
	MinAgeDays int      `json:"minAgeDays"`
	MinSize    int64    `json:"minSize"`
	OnlyBizs   []string `json:"onlyBizs"`
}

// Condition is one leaf of a filter expression (Jira-style):
// {field, operator, value}.
type Condition struct {
	Field string `json:"field"` // biz|sub|category|month|age|size|fileId|reason|thumb|temp
	Op    string `json:"op"`    // eq|ne|in|gt|gte|lt|lte|contains
	Value string `json:"value"`
}

// Expr is a boolean filter tree: AND groups, OR groups and leaf
// conditions. Parentheses in the expression view map to nested groups.
// An empty group matches everything.
type Expr struct {
	And []*Expr    `json:"and,omitempty"` // all children must match
	Or  []*Expr    `json:"or,omitempty"`  // any child must match
	C   *Condition `json:"c,omitempty"`   // leaf condition
}

// Stage is one pipeline function (docs/04 §3): select(kind, ...)（关联展开）、
// order(field, asc|desc)、drop(n) = 跳过前 n 条、take(n) = 取前 n 条。
// 管道按**书写顺序**从左到右组合（函数式）：每个 stage 作用于前一
// stage 的输出——take(10) | select(dup)（先取 10 再展开）与
// select(dup) | take(10)（先展开再取 10）语义不同。
type Stage struct {
	Kind  string   `json:"kind"`            // select|order|drop|take
	Kinds []string `json:"kinds,omitempty"` // select: origin/thumb/dup（正交并集）
	Field string   `json:"field,omitempty"` // order: size|mtime|month|md5
	Desc  bool     `json:"desc,omitempty"`  // order: 降序
	N     int      `json:"n,omitempty"`     // drop/take: 条数
}

// Filter selects a subset of the scanned index. The UI's left tree,
// quick toggles and the filter editor all compile down to one Expr,
// plus an optional pipeline (Stages). Unknown stage kinds fail closed
// (no effect).
type Filter struct {
	Account string  `json:"account"` // instance hash, "" = all
	Expr    *Expr   `json:"expr"`    // nil = everything
	Stages  []Stage `json:"stages,omitempty"`
}

// Sort orders query results.
type Sort struct {
	Field string `json:"field"` // size|mtime|month|md5
	Desc  bool   `json:"desc"`
}

// PageQuery is a paged, filtered, sorted row request (docs/07 §6).
type PageQuery struct {
	Filter   Filter `json:"filter"`
	Sort     Sort   `json:"sort"`
	Page     int    `json:"page"`     // 1-based
	PageSize int    `json:"pageSize"` // default 200, max 500
}

// PageResult is one page of rows.
type PageResult struct {
	Total int              `json:"total"`
	Page  int              `json:"page"`
	Rows  []report.FileRow `json:"rows"`
}

// Stats is a filter's aggregate (file count + bytes).
type Stats struct {
	Count int64 `json:"count"`
	Size  int64 `json:"size"`
}

// DupGroup is one byte-identical content hash with ≥2 copies across the
// whole index: which copy to keep and which copies in the current filter
// are removable. KeepSize/DupSizes 供前端精确累计勾选字节（勾选统计
// 不得依赖虚拟列表已加载行）。
type DupGroup struct {
	Hash         string  `json:"hash"`
	Count        int     `json:"count"`     // 全索引中的总份数
	KeepID       int     `json:"keepId"`    // 建议保留的那份
	KeepLabel    string  `json:"keepLabel"` // 保留份的展示信息（月份/子类型/文件名）
	KeepMTime    int64   `json:"keepMtime"`
	KeepInFilter bool    `json:"keepInFilter"` // 保留份是否在当前筛选内（行内「筛选外」标记用）
	KeepSize     int64   `json:"keepSize"`     // 保留份大小（勾选统计增量用）
	DupIDs       []int   `json:"dupIds"`       // 当前筛选内可删的副本（不含保留份）
	DupSizes     []int64 `json:"dupSizes"`     // 与 DupIDs 对齐的逐份大小
	DupBytes     int64   `json:"dupBytes"`     // 可删副本合计大小
	TotalBytes   int64   `json:"totalBytes"`   // 组内全部大小
}

// CleanRequest comes from the UI; every field is re-validated in Go.
// Audit 与 Move 是确认对话框里的显式 opt-in（默认 false：不生成审计、
// 直接删除）；Move=true 且无 BackupDir 时后端拒绝（docs/06 §3）。
type CleanRequest struct {
	IDs           []int  `json:"ids"`
	BackupDir     string `json:"backupDir"` // Move=true 时使用的备份目录
	Audit         bool   `json:"audit"`     // 生成审计报告（默认关）
	Move          bool   `json:"move"`      // 以移动代替删除（默认关）
	Force         bool   `json:"force"`
	Confirmed     bool   `json:"confirmed"`
	IgnoreRunning bool   `json:"ignoreRunning"` // QQ 运行中仍清理（需二次确认）
}

// CleanItem is one file's outcome in a cleanup run。上层只回传 skip/fail
// 明细（大清理下逐文件列表过长，docs/07 §5）；审计文件（启用时）是
// 完整的权威清单。
type CleanItem struct {
	Path       string `json:"path"`
	Action     string `json:"action"` // move | remove | skip | fail
	BackupPath string `json:"backupPath,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Size       int64  `json:"size"`
}

// CleanResult summarizes a cleanup run.
type CleanResult struct {
	Processed  int         `json:"processed"`
	Moved      int         `json:"moved"`
	Deleted    int         `json:"deleted"`
	Skipped    int         `json:"skipped"`
	Failed     int         `json:"failed"`
	BytesFreed int64       `json:"bytesFreed"`
	Items      []CleanItem `json:"items"`     // 仅 skip/fail 明细
	AuditPath  string      `json:"auditPath"` // 本次审计报告（按需生成；未启用时为空）
	Errors     []string    `json:"errors"`
}
