// Package clean executes deletions. Every safety redline (docs/06) is
// enforced here, in Go — the UI is treated as untrusted input.
//
// Required order per deletion: process guard → force flag → explicit
// confirmation → whitelist + blacklist re-verification → backup/delete
// → audit record（按需，docs/06 §3：默认不生成审计文件）.
package clean

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"qqcleaner/internal/classify"
	"qqcleaner/internal/platform"
	"qqcleaner/internal/rules"
)

var (
	ErrNotForced    = errors.New("cleanup requires --force (redline: no unforced deletion)")
	ErrNotConfirmed = errors.New("cleanup requires explicit confirmation")
	ErrQQRunning    = errors.New("QQ is running; refusing to clean (close QQ first)")
)

// Request carries everything a cleanup run needs. Every field that weakens
// safety (Force, Confirmed) is explicit; Move 与 AuditLog 均为**显式
// opt-in**（默认直接删除、默认不生成审计文件，docs/06 §3）。
type Request struct {
	Files        []classify.FileEntry
	AllowedRoots []string // nt_data roots the files must live under
	BackupDir    string   // Move=true 时的备份目录（Move=false 时忽略）
	Move         bool     // 显式移动代替删除；true 且无 BackupDir → 拒绝
	AuditLog     string   // JSONL 审计路径；空 = 不生成审计（opt-in）
	Force        bool
	Confirmed    bool
	// IgnoreRunning 允许在 QQ 运行中清理（产品决策：POSIX 下 unlink 与
	// 并发写互不锁定；残余风险仅是缓存条目失效可重新生成。默认仍拒绝，
	// 需用户在确认对话框显式选择「仍要清理」）。
	IgnoreRunning bool
	// K 是 QQ 知识实现（白名单/黑名单结构 + 打分），必需。
	K      rules.Knowledge
	Config rules.Config
	Now    time.Time // zero = time.Now()
}

// CleanItem is one file's outcome in a cleanup run。审计文件（启用时）
// 是完整权威清单；Items 供 UI 回显（上层只回传 skip/fail 明细与统计，
// 大清理下逐文件列表过长——docs/07 §5）。
type CleanItem struct {
	Path       string `json:"path"`
	Action     string `json:"action"` // move | remove | skip | fail
	BackupPath string `json:"backupPath,omitempty"`
	Reason     string `json:"reason,omitempty"` // skip/fail 的原因
	Size       int64  `json:"size"`
}

// Result summarizes a run. Errors are collected per-file so one failure
// never aborts the rest (docs/04 §4.4).
type Result struct {
	Processed  int
	Deleted    int // removed without backup
	Moved      int // moved to backup dir
	Skipped    int // failed whitelist/blacklist verification
	Failed     int
	BytesFreed int64
	Items      []CleanItem
	Errors     []string
}

// Run executes the deletion pass. It refuses to run at all unless Force and
// Confirmed are set and QQ is not running; per-file it re-verifies the
// whitelist/blacklist, moves to the backup dir (explicit Move) or removes,
// and appends an audit record only when an audit log was requested.
func Run(ctx context.Context, req Request) (Result, error) {
	if !req.Force {
		return Result{}, ErrNotForced
	}
	if !req.Confirmed {
		return Result{}, ErrNotConfirmed
	}
	if req.Move && req.BackupDir == "" {
		return Result{}, fmt.Errorf("move requested but no backup dir configured (set one first)")
	}
	if req.K == nil {
		return Result{}, fmt.Errorf("knowledge implementation is required (fail-closed)")
	}
	if !req.IgnoreRunning && qqRunningFunc() {
		return Result{}, qqRunningError()
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}

	var audit *auditLogger
	if req.AuditLog != "" {
		a, err := openAuditLog(req.AuditLog)
		if err != nil {
			return Result{}, fmt.Errorf("open audit log: %w", err)
		}
		audit = a
		defer audit.Close()
	}
	backupDir := ""
	if req.Move {
		backupDir = req.BackupDir
	}

	// Errors 初始化为空切片（JSON 序列化为 [] 而非 null）：前端契约
	// 是数组，nil 切片经 Wails 传到 JS 会变成 null 引发 TypeError。
	res := Result{Errors: []string{}, Items: []CleanItem{}}
	lastQQCheck := time.Now()
	for _, f := range req.Files {
		if ctx.Err() != nil {
			break
		}
		res.Processed++
		// Process guard re-checked close to every deletion (docs/06 §5)；
		// IgnoreRunning 时同样跳过（用户已显式确认）。
		if !req.IgnoreRunning && time.Since(lastQQCheck) >= 30*time.Second {
			if qqRunningFunc() {
				res.Errors = append(res.Errors, "QQ started during cleanup; aborting")
				return res, qqRunningError()
			}
			lastQQCheck = time.Now()
		}
		// Whitelist/blacklist re-verification (docs/06 §2): the scan result
		// is not trusted, every path is checked again right here.
		if err := VerifyPath(req.K, f.Path, req.AllowedRoots, req.Config); err != nil {
			res.Skipped++
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", f.Path, err))
			res.Items = append(res.Items, CleanItem{Path: f.Path, Action: "skip", Reason: err.Error(), Size: f.Size})
			continue
		}

		// 审计记录附带 reason 标签：关联标签（重复出现/原图仍在/有缩略图）
		// 需要全量扫描邻域，clean 层单文件语境下保守置空——审计只承载
		// 类别标签，不信任扫描期关联结果。
		reason := rules.Reason(f, false, false, 0)

		backupPath, err := deleteOne(audit, f, backupDir, reason)
		if err != nil {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", f.Path, err))
			res.Items = append(res.Items, CleanItem{Path: f.Path, Action: "fail", Reason: err.Error(), Size: f.Size})
			continue
		}
		action := "remove"
		if req.Move {
			res.Moved++
			action = "move"
		} else {
			res.Deleted++
		}
		res.BytesFreed += f.Size
		res.Items = append(res.Items, CleanItem{Path: f.Path, Action: action, BackupPath: backupPath, Reason: reason, Size: f.Size})
	}
	return res, nil
}

// structuralRel enforces the structural red lines on an absolute path —
// under an allowed nt_data root, no traversal, not blacklisted (状态目录/
// db 后缀) — and returns the root-relative path.
func structuralRel(k rules.Knowledge, abs string, allowedRoots []string) (string, error) {
	cleaned := filepath.Clean(abs)
	var under string
	for _, root := range allowedRoots {
		r := filepath.Clean(root)
		if cleaned == r || strings.HasPrefix(cleaned, r+string(filepath.Separator)) {
			under = r
			break
		}
	}
	if under == "" {
		return "", fmt.Errorf("outside allowed roots")
	}
	rel, err := filepath.Rel(under, cleaned)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path traversal blocked")
	}
	if rules.Blacklisted(k, cleaned) {
		return "", fmt.Errorf("blacklisted path")
	}
	return rel, nil
}

// VerifyStructural enforces the structural red lines only (no category
// gates). 预览/揭示用：可清性门控（emoji 等类别开关）不应阻止查看
// keep 级（report-only）条目 —— 路径本身来自扫描索引，非前端输入。
func VerifyStructural(k rules.Knowledge, abs string, allowedRoots []string) error {
	_, err := structuralRel(k, abs, allowedRoots)
	return err
}

// VerifyPath = VerifyStructural + 白名单（删除前的逐文件重验，docs/06 §5b）。
func VerifyPath(k rules.Knowledge, abs string, allowedRoots []string, cfg rules.Config) error {
	rel, err := structuralRel(k, abs, allowedRoots)
	if err != nil {
		return err
	}
	if !rules.Whitelisted(k, rel, cfg) {
		return fmt.Errorf("not in the clean whitelist (structure or category gate)")
	}
	return nil
}

// deleteOne moves the file to the backup dir (recoverable) or removes it.
// 删除/移动的 OS 语义由 platform 适配层提供（POSIX unlink 与 Windows
// DeleteFile 不同）。审计按需：audit 为 nil 时只执行不记录（docs/06 §3）。
// 返回备份路径（move 时非空，remove 时为 ""）。
func deleteOne(audit *auditLogger, f classify.FileEntry, backupDir, reason string) (string, error) {
	entry := auditEntry{
		Path:   f.Path,
		Size:   f.Size,
		Reason: reason,
	}
	if backupDir != "" {
		dst := filepath.Join(backupDir, filepath.Base(f.Path))
		if _, err := os.Lstat(dst); err == nil {
			dst = uniquePath(dst) // avoid clobbering an existing backup
		}
		if err := platform.Current().MoveFile(f.Path, dst); err != nil {
			return "", err
		}
		entry.Action, entry.BackupPath = "move", dst
		if audit != nil {
			if err := audit.Log(entry); err != nil {
				return dst, err
			}
		}
		return dst, nil
	}
	// 产品决策：删除前不再计算 SHA-256（对已删文件无恢复价值，
	// 且大文件全量哈希拖慢清理）；需要可恢复性请勾选移动（备份目录）。
	if err := platform.Current().DeleteFile(f.Path); err != nil {
		return "", err
	}
	entry.Action = "remove"
	if audit != nil {
		if err := audit.Log(entry); err != nil {
			return "", err
		}
	}
	return "", nil
}

func uniquePath(p string) string {
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s.%d", p, i)
		if _, err := os.Lstat(cand); err != nil {
			return cand
		}
	}
}
