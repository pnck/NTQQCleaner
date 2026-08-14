package clean

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// auditEntry is one JSONL record written for every deleted file
// (docs/06 §3: deleting without a record is a bug)。SHA256 字段保留
// 为可选（默认不计算——见 clean.go deleteOne 的产品决策注释）。
type auditEntry struct {
	Time       string `json:"time"`
	Action     string `json:"action"` // move | remove
	Path       string `json:"path"`
	BackupPath string `json:"backupPath,omitempty"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256,omitempty"`
	Reason     string `json:"reason"`
}

// maxAuditBytes 是审计日志的轮转上限（1 MiB）：工具自身不留垃圾——
// 超出后旧日志滚动为 <path>.1（覆盖上一轮），只保留最近两份。
const maxAuditBytes = 1 << 20

// auditLogger appends JSONL records to a log file (created on demand).
type auditLogger struct {
	f    *os.File
	path string
}

func openAuditLog(path string) (*auditLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &auditLogger{f: f, path: path}, nil
}

func (a *auditLogger) Close() error { return a.f.Close() }

func (a *auditLogger) rotate() error {
	if err := a.f.Close(); err != nil {
		return err
	}
	if err := os.Rename(a.path, a.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	a.f = f
	return nil
}

func (a *auditLogger) Log(e auditEntry) error {
	if st, err := a.f.Stat(); err == nil && st.Size() >= maxAuditBytes {
		if err := a.rotate(); err != nil {
			return err
		}
	}
	e.Time = time.Now().Format(time.RFC3339)
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.f, "%s\n", b)
	return err
}
