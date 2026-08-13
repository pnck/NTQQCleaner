// Package report holds the scan/report models shared by the CLI and the UI
// backend. Frontend-facing rows deliberately exclude absolute paths:
// previews go through an ID-based endpoint (docs/04 §5, docs/07 §6).
package report

// TierTotals aggregates bytes per tier label.
type TierTotals struct {
	Safe    int64 `json:"safe"`
	Suggest int64 `json:"suggest"`
	Caution int64 `json:"caution"`
	Keep    int64 `json:"keep"`
}

// Add accumulates one file.
func (t *TierTotals) Add(tier string, size int64) {
	switch tier {
	case "safe":
		t.Safe += size
	case "suggest":
		t.Suggest += size
	case "caution":
		t.Caution += size
	case "keep":
		t.Keep += size
	}
}

// Sum returns the total across all tiers.
func (t TierTotals) Sum() int64 { return t.Safe + t.Suggest + t.Caution + t.Keep }

// AccountReport is the per-account scan summary sent to the UI.
type AccountReport struct {
	Hash        string                `json:"hash"`
	QQNum       string                `json:"qqNum"` // "" = unknown
	NtData      string                `json:"ntData"`
	LatestMonth string                `json:"latestMonth"`
	TotalFiles  int                   `json:"totalFiles"`
	TotalSize   int64                 `json:"totalSize"`
	Totals      TierTotals            `json:"totals"`
	ByBiz       map[string]TierTotals `json:"byBiz"` // key = biz (pic/video/...)
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
	Tier     string `json:"tier"`
	Reason   string `json:"reason"`
	ThumbURL string `json:"thumbUrl"`
	OriURL   string `json:"oriUrl"`
	OriExt   string `json:"oriExt"` // 配对原文件的扩展名（预览端据此分派播放器）；无原文件为空
}

// GroupStat is an aggregation bucket for the left tree (biz × month).
type GroupStat struct {
	Key    string     `json:"key"` // biz name or "YYYY-MM"
	Count  int        `json:"count"`
	Size   int64      `json:"size"`
	Totals TierTotals `json:"totals"`
}

// Manifest is the CLI bridge between `scan --json` and `clean --file`
// (docs/04 §8). It carries enough for clean to re-verify every file.
type Manifest struct {
	Version  int               `json:"version"`
	Root     string            `json:"root"`
	Config   ManifestConfig    `json:"config"`
	Accounts []ManifestAccount `json:"accounts"`
}

// ManifestConfig records the scoring config that produced the manifest.
type ManifestConfig struct {
	ThresholdSeconds int64 `json:"thresholdSeconds"`
	Aggressive       bool  `json:"aggressive"`
	Safe             int   `json:"safe"`
	Suggest          int   `json:"suggest"`
	Caution          int   `json:"caution"`
}

// ManifestAccount is per-account entries with tiers attached.
type ManifestAccount struct {
	Hash    string          `json:"hash"`
	QQNum   string          `json:"qqNum"`
	NtData  string          `json:"ntData"`
	Entries []ManifestEntry `json:"entries"`
}

// ManifestEntry is a classified file plus its tier/reason at scan time.
type ManifestEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	MTime  int64  `json:"mtime"`
	MD5    string `json:"md5"`
	Biz    string `json:"biz"`
	Sub    string `json:"sub"`
	Month  string `json:"month"`
	Tier   string `json:"tier"`
	Reason string `json:"reason"`
}
