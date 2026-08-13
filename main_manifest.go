package main

import (
	"context"
	"fmt"
	"path/filepath"

	"qqcleaner/internal/classify"
	"qqcleaner/internal/clean"
	"qqcleaner/internal/qq"
	_ "qqcleaner/internal/qqimpl" // 注册 probe（QQ 平台×版本 dispatcher）
	"qqcleaner/internal/report"
	"qqcleaner/internal/rules"
)

// multiFlag collects repeatable flag values.
type multiFlag struct{ list []string }

func (m *multiFlag) String() string { return "[]" }

func (m *multiFlag) Set(v string) error {
	m.list = append(m.list, v)
	return nil
}

// classifyEntry is the subset of a manifest entry the clean layer needs.
type classifyEntry struct {
	path  string
	size  int64
	mtime int64
	md5   string
	biz   string
	sub   string
	month string
}

// cleanRun converts manifest entries back into classify.FileEntry and runs
// the shared clean package (the same redlines as the GUI path).
func cleanRun(m report.Manifest, files []classifyEntry, backupDir, auditLog string, cfg rules.Config, ignoreRunning bool) (clean.Result, error) {
	entries := make([]classify.FileEntry, 0, len(files))
	for _, f := range files {
		entries = append(entries, classify.FileEntry{
			Path:  f.path,
			Size:  f.size,
			MTime: f.mtime,
			MD5:   f.md5,
			Biz:   f.biz,
			Sub:   f.sub,
			Month: f.month,
		})
	}
	// Allowed roots are the per-account nt_data dirs from the manifest,
	// re-anchored under the manifest root so a moved/edited manifest can
	// never widen the deletion surface.
	var allowed []string
	for _, a := range m.Accounts {
		ntData := a.NtData
		if !filepath.IsAbs(ntData) {
			ntData = filepath.Join(m.Root, ntData)
		}
		allowed = append(allowed, filepath.Clean(ntData))
	}
	// 从清单根目录重新分派知识实现（与 GUI 同一 dispatcher）。
	k := qq.Detect(m.Root)
	if !k.ScanCapable() {
		return clean.Result{}, fmt.Errorf("unsupported QQ data layout (detected: %s); cleaning disabled", k.Name())
	}
	return clean.Run(context.Background(), clean.Request{
		Files:         entries,
		AllowedRoots:  allowed,
		BackupDir:     backupDir,
		AuditLog:      auditLog,
		Force:         true, // gated by the --force flag in cleanCmd
		Confirmed:     true, // gated by the interactive `yes` in cleanCmd
		IgnoreRunning: ignoreRunning,
		K:             k,
		Config:        cfg,
	})
}
