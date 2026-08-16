// Package rules implements the whitelist/blacklist path policy and the
// reason-label model. It is pure logic: no I/O beyond config load/save.
//
// Reference: docs/03_clean_rules.md, docs/06_safety_redlines.md
package rules

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// QQOfficialThresholdSeconds = 259199 ≈ 3 days：QQ 官方清理基线（docs/03
// §1）。扫描的 MinAgeDays 默认值（DefaultMinAgeDays）对齐该基线。
const QQOfficialThresholdSeconds int64 = 259199

// DefaultMinAgeDays is the scan default for MinAgeDays: files newer than
// this are never even listed (ceil(259199s / 86400s) = 3, docs/03 §1).
const DefaultMinAgeDays = 3

// Config holds every tunable of the cleanability policy (docs/03 §6). Zero
// values mean "use Default()"; Load fills missing fields from defaults.
type Config struct {
	// Category gates: which kinds of files participate in cleanability at all
	// (docs/03 §6). "false" = report only, never cleanable.
	CleanTemp          bool `yaml:"clean_temp" json:"cleanTemp"`
	CleanThumb         bool `yaml:"clean_thumb" json:"cleanThumb"`
	CleanOri           bool `yaml:"clean_ori" json:"cleanOri"`
	CleanBaseEmoji     bool `yaml:"clean_base_emoji" json:"cleanBaseEmoji"`
	CleanMarketface    bool `yaml:"clean_marketface" json:"cleanMarketface"`
	CleanPersonalEmoji bool `yaml:"clean_personal_emoji" json:"cleanPersonalEmoji"`
	CleanFile          bool `yaml:"clean_file" json:"cleanFile"`
	// 高级 opt-in 门控（默认关，设置里勾选后扫描与清理才覆盖，docs/03 §6）：
	CleanLog         bool `yaml:"clean_log" json:"cleanLog"`                  // log/log-cache 运行日志（QQ 自动重建）
	CleanDatalineTmp bool `yaml:"clean_dataline_tmp" json:"cleanDatalineTmp"` // dataline/.tmp 传输残留（NFC）
	CleanAvatar      bool `yaml:"clean_avatar" json:"cleanAvatar"`            // avatar 头像缓存（重新拉取）
	// ScanAllAges 解开 QQ 官方 3 天扫描基线（逆向结论的默认对齐，非产品
	// 需求，docs/03 §1）：true = 不设最小年龄，近 3 天文件同样入索引。
	// 设置→高级 opt-in，默认 false（保持 QQ 3 天基线）。
	ScanAllAges bool `yaml:"scan_all_ages" json:"scanAllAges"`

	MinFileSizeBytes int64    `yaml:"min_file_size_bytes" json:"minFileSizeBytes"`
	SkipDirs         []string `yaml:"skip_dirs" json:"skipDirs"`
	// BackupDir 清理时把文件移动到的备份目录（GUI 设置项；空 = 直接删除，
	// 仅写审计报告）。与其它设置一同存在 tmp 下的 config.yaml。
	BackupDir string `yaml:"backup_dir" json:"backupDir"`

	// 面板布局（拖拽记忆，docs/07 §2）：左右栏宽 + 左栏 biz 分区高 +
	// 预览媒体区高。0 = 未持久化过，前端用默认值。与其它设置同存于
	// tmp 下的 config.yaml（拖拽结束写回）。
	LeftPanelWidth     int `yaml:"left_panel_width" json:"leftPanelWidth"`
	RightPanelWidth    int `yaml:"right_panel_width" json:"rightPanelWidth"`
	BizTreeHeight      int `yaml:"biz_tree_height" json:"bizTreeHeight"`
	PreviewMediaHeight int `yaml:"preview_media_height" json:"previewMediaHeight"`

	// 前端持久化状态（Go 不解释，只保管——WebView 自带存储弃用：
	// localStorage 落在 WebView profile 目录，app 退出后清理困难）：
	Theme         string `yaml:"theme" json:"theme"`                  // auto / light / dark
	NamedFilters  string `yaml:"named_filters" json:"namedFilters"`   // 命名筛选器列表的 JSON 载荷
	FiltersSeeded string `yaml:"filters_seeded" json:"filtersSeeded"` // 内置筛选器种子版本标记
}

// Default returns the documented defaults (docs/03 §6).
func Default() Config {
	return Config{
		CleanTemp:          true,
		CleanThumb:         true,
		CleanOri:           false,
		CleanBaseEmoji:     false,
		CleanMarketface:    false,
		CleanPersonalEmoji: false,
		CleanFile:          false,
		CleanLog:           false,
		CleanDatalineTmp:   false,
		CleanAvatar:        false,
		ScanAllAges:        false,
		MinFileSizeBytes:   0,
		SkipDirs:           []string{"mmkv", "msf", "OnlineStatus", "UnitedConfig", "config"},
	}
}

// Load reads a YAML config, overlaying it on Default() so that partial files
// keep every other default.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Save writes the config atomically (temp file + rename) so a crash can
// never truncate the user's settings.
func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Validate rejects configs that would violate the safety model.
func (c *Config) Validate() error {
	if c.MinFileSizeBytes < 0 {
		return fmt.Errorf("min_file_size_bytes must be >= 0")
	}
	if c.LeftPanelWidth < 0 || c.RightPanelWidth < 0 || c.BizTreeHeight < 0 || c.PreviewMediaHeight < 0 {
		return fmt.Errorf("panel layout values must be >= 0")
	}
	return nil
}

// SkipDirSet converts SkipDirs to a set for fast lookup.
func (c Config) SkipDirSet() map[string]bool {
	m := make(map[string]bool, len(c.SkipDirs))
	for _, d := range c.SkipDirs {
		m[d] = true
	}
	return m
}
