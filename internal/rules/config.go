// Package rules implements the value-scoring model and the whitelist/
// blacklist path rules. It is pure logic: no I/O beyond config load/save.
//
// Reference: docs/03_clean_rules.md, docs/06_safety_redlines.md
package rules

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// QQOfficialThresholdSeconds = 259199 ≈ 3 days. Files newer than this are
// never cleanable (QQ's own baseline, docs/03 §1)。这是默认值；知识实现
// 可在扫描初始化时按版本族调整 DefaultThresholdSeconds。
const QQOfficialThresholdSeconds int64 = 259199

// ScoreThresholds maps the 0-100 value score to tier labels (docs/03 §4).
type ScoreThresholds struct {
	Safe    int `yaml:"safe" json:"safe"`
	Suggest int `yaml:"suggest" json:"suggest"`
	Caution int `yaml:"caution" json:"caution"`
}

// Config holds every tunable of the scoring model (docs/03 §7). Zero values
// mean "use Default()"; Load fills missing fields from defaults.
type Config struct {
	DefaultThresholdSeconds     int64           `yaml:"default_threshold_seconds" json:"defaultThresholdSeconds"`
	TimeTierDays                []int64         `yaml:"time_tier_days" json:"timeTierDays"` // [30,90,180,365]
	ArchiveMonthOlderThanMonths int             `yaml:"archive_month_older_than_months" json:"archiveMonthOlderThanMonths"`
	ScoreThresholds             ScoreThresholds `yaml:"score_thresholds" json:"scoreThresholds"`
	Aggressive                  bool            `yaml:"aggressive" json:"aggressive"` // include 🟠 caution tier

	// Category gates: which kinds of files participate in cleanability at all
	// (docs/03 §7). "false" = report only, never listed as cleanable.
	CleanTemp          bool `yaml:"clean_temp" json:"cleanTemp"`
	CleanThumb         bool `yaml:"clean_thumb" json:"cleanThumb"`
	CleanOri           bool `yaml:"clean_ori" json:"cleanOri"`
	CleanBaseEmoji     bool `yaml:"clean_base_emoji" json:"cleanBaseEmoji"`
	CleanMarketface    bool `yaml:"clean_marketface" json:"cleanMarketface"`
	CleanPersonalEmoji bool `yaml:"clean_personal_emoji" json:"cleanPersonalEmoji"`
	CleanFile          bool `yaml:"clean_file" json:"cleanFile"`
	CleanLog           bool `yaml:"clean_log" json:"cleanLog"`
	CleanAvatar        bool `yaml:"clean_avatar" json:"cleanAvatar"`

	MinFileSizeBytes int64    `yaml:"min_file_size_bytes" json:"minFileSizeBytes"`
	SkipDirs         []string `yaml:"skip_dirs" json:"skipDirs"`
}

// Default returns the documented defaults (docs/03 §7).
func Default() Config {
	return Config{
		DefaultThresholdSeconds:     QQOfficialThresholdSeconds,
		TimeTierDays:                []int64{30, 90, 180, 365},
		ArchiveMonthOlderThanMonths: 12,
		ScoreThresholds:             ScoreThresholds{Safe: 30, Suggest: 55, Caution: 75},
		CleanTemp:                   true,
		CleanThumb:                  true,
		CleanOri:                    false,
		CleanBaseEmoji:              false,
		CleanMarketface:             false,
		CleanPersonalEmoji:          false,
		CleanFile:                   false,
		CleanLog:                    false,
		CleanAvatar:                 false,
		MinFileSizeBytes:            0,
		SkipDirs:                    []string{"mmkv", "msf", "OnlineStatus", "UnitedConfig", "config", "log", "log-cache", "avatar"},
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

// Validate rejects configs whose thresholds would violate the safety model
// (negative ages, safe > suggest > caution, ...).
func (c *Config) Validate() error {
	if c.DefaultThresholdSeconds < 0 {
		return fmt.Errorf("default_threshold_seconds must be >= 0")
	}
	if c.ScoreThresholds.Safe < 0 || c.ScoreThresholds.Suggest <= c.ScoreThresholds.Safe || c.ScoreThresholds.Caution <= c.ScoreThresholds.Suggest {
		return fmt.Errorf("score_thresholds must satisfy 0 <= safe < suggest < caution <= 100")
	}
	if c.ScoreThresholds.Caution > 100 {
		return fmt.Errorf("score_thresholds.caution must be <= 100")
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
