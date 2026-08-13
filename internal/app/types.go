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
	Aggressive bool     `json:"aggressive"`
}

// Condition is one leaf of a filter expression (Jira-style):
// {field, operator, value}.
type Condition struct {
	Field string `json:"field"` // biz|sub|category|month|age|size|md5|reason|thumb|temp|tier
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

// OrderStage is one order() pipeline function: sort by field asc/desc.
// Multiple stages sort stably in sequence (= 多关键字排序).
type OrderStage struct {
	Field string `json:"field"` // size|mtime|month|md5|tier
	Desc  bool   `json:"desc"`
}

// Filter selects a subset of the scanned index. The UI's left tree,
// quick toggles and the filter editor all compile down to one Expr,
// plus optional pipeline stages: order(field, asc|desc)、
// drop(n) = 跳过前 n 条、take(n) = 取排序后前 n 条。
// 语义顺序：order → drop → take（排序在 take/drop 之前，筛选器自包含）。
type Filter struct {
	Account string       `json:"account"` // instance hash, "" = all
	Expr    *Expr        `json:"expr"`    // nil = everything
	Orders  []OrderStage `json:"orders,omitempty"`
	Limit   int          `json:"limit,omitempty"`  // take(n)
	Offset  int          `json:"offset,omitempty"` // drop(n)
}

// Sort orders query results.
type Sort struct {
	Field string `json:"field"` // size|mtime|tier|md5
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

// DupGroup is one md5 with ≥2 copies across the whole index: which copy
// to keep and which copies in the current filter are removable.
type DupGroup struct {
	MD5        string `json:"md5"`
	Count      int    `json:"count"`      // 全索引中的总份数
	KeepID     int    `json:"keepId"`     // 建议保留的那份
	KeepLabel  string `json:"keepLabel"`  // 保留份的展示信息（月份/子类型/文件名）
	KeepMTime  int64  `json:"keepMtime"`
	DupIDs     []int  `json:"dupIds"`     // 当前筛选内可删的副本（不含保留份）
	DupBytes   int64  `json:"dupBytes"`   // 可删副本合计大小
	TotalBytes int64  `json:"totalBytes"` // 组内全部大小
}

// CleanRequest comes from the UI; every field is re-validated in Go.
type CleanRequest struct {
	IDs       []int  `json:"ids"`
	BackupDir string `json:"backupDir"`
	Force     bool   `json:"force"`
	Confirmed bool   `json:"confirmed"`
}

// CleanResult summarizes a cleanup run.
type CleanResult struct {
	Processed  int      `json:"processed"`
	Moved      int      `json:"moved"`
	Deleted    int      `json:"deleted"`
	Skipped    int      `json:"skipped"`
	Failed     int      `json:"failed"`
	BytesFreed int64    `json:"bytesFreed"`
	Errors     []string `json:"errors"`
}
