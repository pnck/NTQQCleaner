// Package clean executes deletions. Every safety redline (docs/06) is
// enforced here, in Go — the UI is treated as untrusted input.
//
// Required order per deletion: process guard → force flag → explicit
// confirmation → whitelist + blacklist re-verification → backup/hash →
// delete → audit record.
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
// safety (Force, Confirmed, BackupDir=="" → hash audit) is explicit.
type Request struct {
	Files        []classify.FileEntry
	AllowedRoots []string // nt_data roots the files must live under
	BackupDir    string   // non-empty = move files there (recoverable)
	AuditLog     string   // JSONL audit log path (always written)
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

// Result summarizes a run. Errors are collected per-file so one failure
// never aborts the rest (docs/04 §4.4).
type Result struct {
	Processed  int
	Deleted    int // removed without backup
	Moved      int // moved to backup dir
	Skipped    int // failed whitelist/blacklist verification
	Failed     int
	BytesFreed int64
	Errors     []string
}

// Run executes the deletion pass. It refuses to run at all unless Force and
// Confirmed are set and QQ is not running; per-file it re-verifies the
// whitelist/blacklist, moves to the backup dir (or removes), and appends an
// audit record for every single file.
func Run(ctx context.Context, req Request) (Result, error) {
	if !req.Force {
		return Result{}, ErrNotForced
	}
	if !req.Confirmed {
		return Result{}, ErrNotConfirmed
	}
	if req.AuditLog == "" {
		return Result{}, fmt.Errorf("audit log path is required (redline: no unrecorded deletion)")
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

	audit, err := openAuditLog(req.AuditLog)
	if err != nil {
		return Result{}, fmt.Errorf("open audit log: %w", err)
	}
	defer audit.Close()

	var res Result
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
			continue
		}

		// Re-score at clean time with a single-entry index: redundancy
		// bonuses need the full scan neighborhood, so this is conservative
		// (lower scores → stricter tiers). The clean layer never trusts the
		// scan-time tier.
		idx := rules.BuildMD5Index([]classify.FileEntry{f})
		score := rules.Score(req.K, f, idx, req.Config, now)
		tier := rules.Tier(f, score, req.Config, now)
		reason := rules.Reason(f, tier, idx)

		if err := deleteOne(audit, f, req.BackupDir, tier, reason); err != nil {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", f.Path, err))
			continue
		}
		if req.BackupDir != "" {
			res.Moved++
		} else {
			res.Deleted++
		}
		res.BytesFreed += f.Size
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
		return fmt.Errorf("not whitelisted")
	}
	return nil
}

// deleteOne moves the file to the backup dir (recoverable) or removes it.
// 删除/移动的 OS 语义由 platform 适配层提供（POSIX unlink 与 Windows
// DeleteFile 不同）。Every outcome is audited (docs/06 §3).
func deleteOne(audit *auditLogger, f classify.FileEntry, backupDir, tier, reason string) error {
	entry := auditEntry{
		Path:   f.Path,
		Size:   f.Size,
		Tier:   tier,
		Reason: reason,
	}
	if backupDir != "" {
		dst := filepath.Join(backupDir, filepath.Base(f.Path))
		if _, err := os.Lstat(dst); err == nil {
			dst = uniquePath(dst) // avoid clobbering an existing backup
		}
		if err := platform.Current().MoveFile(f.Path, dst); err != nil {
			return err
		}
		entry.Action, entry.BackupPath = "move", dst
	} else {
		// 产品决策：删除前不再计算 SHA-256（对已删文件无恢复价值，
		// 且大文件全量哈希拖慢清理）；审计日志记录路径/大小/时间/分级。
		// 需要可恢复性请配置备份目录（移动而非删除）。
		if err := platform.Current().DeleteFile(f.Path); err != nil {
			return err
		}
		entry.Action = "remove"
	}
	return audit.Log(entry)
}

func uniquePath(p string) string {
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s.%d", p, i)
		if _, err := os.Lstat(cand); err != nil {
			return cand
		}
	}
}
