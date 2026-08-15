// Package report holds the scan/report models shared by the CLI and the UI
// backend. Frontend-facing rows deliberately exclude absolute paths:
// previews go through an ID-based endpoint (docs/04 §5, docs/07 §6).
package report

// AccountReport is the per-account scan summary sent to the UI.
type AccountReport struct {
	Hash        string `json:"hash"`
	QQNum       string `json:"qqNum"` // "" = unknown
	NtData      string `json:"ntData"`
	LatestMonth string `json:"latestMonth"`
	TotalFiles  int    `json:"totalFiles"`
	TotalSize   int64  `json:"totalSize"`
	// HashedFiles = 参与内容哈希（大小冲突候选）的文件数：二次扫描是否
	// 生效的直接证据（0 表示本账号没有任何同大小文件对）。
	HashedFiles int `json:"hashedFiles"`
	// 旧版残留（docs/08 §3.5）：只统计、永不清理。Windows NT 账号的旧库
	// （Msg3.0.db 等）或旧版账号的整目录占用。
	LegacyResidueSize  int64 `json:"legacyResidueSize"`
	LegacyResidueCount int   `json:"legacyResidueCount"`
}

// FileRow is one row in the UI's file browser (docs/07 §6). ThumbURL/OriURL
// point at /preview/{id} endpoints resolved server-side.
type FileRow struct {
	ID       int    `json:"id"`
	MD5      string `json:"md5"`
	Biz      string `json:"biz"`
	Sub      string `json:"sub"`
	Month    string `json:"month"`
	Size     int64  `json:"size"`
	MTime    int64  `json:"mtime"`
	Ext      string `json:"ext"` // 文件扩展名（角标展示）
	Reason   string `json:"reason"`
	ThumbURL string `json:"thumbUrl"`
	OriURL   string `json:"oriUrl"`
	OriExt   string `json:"oriExt"` // 配对原文件的扩展名（预览端据此分派播放器）；无原文件为空
	// ContentHash 是文件字节的 SHA-256（二次扫描）；仅当存在同大小
	// 其它文件时才会计算，大小唯一时为空。
	ContentHash string `json:"contentHash"`
	// ContentDupCount 是字节级相同内容的副本总数（含自身；≥2 即重复出现）。
	ContentDupCount int `json:"contentDupCount"`
}

// GroupStat is an aggregation bucket for the left tree (biz × month).
type GroupStat struct {
	Key   string `json:"key"` // biz name or "YYYY-MM"
	Count int    `json:"count"`
	Size  int64  `json:"size"`
}

// Manifest is the CLI bridge between `scan --json` and `clean --file`
// (docs/04 §8). It carries enough for clean to re-verify every file.
type Manifest struct {
	Version  int               `json:"version"`
	Root     string            `json:"root"`
	Accounts []ManifestAccount `json:"accounts"`
}

// ManifestAccount is per-account entries with their reason labels.
type ManifestAccount struct {
	Hash    string          `json:"hash"`
	QQNum   string          `json:"qqNum"`
	NtData  string          `json:"ntData"`
	Entries []ManifestEntry `json:"entries"`
}

// ManifestEntry is a classified file plus its reason at scan time.
type ManifestEntry struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	MTime       int64  `json:"mtime"`
	MD5         string `json:"md5"`
	Biz         string `json:"biz"`
	Sub         string `json:"sub"`
	Month       string `json:"month"`
	Reason      string `json:"reason"`
	ContentHash string `json:"contentHash,omitempty"`
}
