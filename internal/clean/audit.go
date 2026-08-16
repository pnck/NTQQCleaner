package clean

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// auditEntry is one JSONL record written for a deleted/moved file when
// auditing was requested（docs/06 §3：审计按需，默认不生成）。SHA256
// 字段保留为可选（默认不计算——见 clean.go deleteOne 的产品决策注释）。
type auditEntry struct {
	Time       string `json:"time"`
	Action     string `json:"action"` // move | remove | reboot（docs/09 §3.1）
	Path       string `json:"path"`
	BackupPath string `json:"backupPath,omitempty"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256,omitempty"`
	Reason     string `json:"reason"`
}

// auditLogger appends JSONL records to a log file (created on demand).
// 审计文件是每次清理生成的一份**临时报告**（位于系统 tmp），清理后
// 由调用方打开供用户查看/另存——不轮转、不积累（工具自身不留垃圾）。
type auditLogger struct {
	f *os.File
}

func openAuditLog(path string) (*auditLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &auditLogger{f: f}, nil
}

func (a *auditLogger) Close() error { return a.f.Close() }

func (a *auditLogger) Log(e auditEntry) error {
	e.Time = time.Now().Format(time.RFC3339)
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.f, "%s\n", b)
	return err
}
