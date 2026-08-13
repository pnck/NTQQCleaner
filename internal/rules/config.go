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

	MinFileSizeBytes int64    `yaml:"min_file_size_bytes" json:"minFileSizeBytes"`
	SkipDirs         []string `yaml:"skip_dirs" json:"skipDirs"`
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
		MinFileSizeBytes:   0,
		SkipDirs:           []string{"mmkv", "msf", "OnlineStatus", "UnitedConfig", "config", "log", "log-cache", "avatar"},
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
