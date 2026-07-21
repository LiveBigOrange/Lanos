// Package config loads and persists user settings to config.yaml.
// See PRD §3.5 and §5.3.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk user configuration. Fields are intentionally
// public for yaml round-trip; mutators should go through Set* methods
// to keep the in-memory copy consistent and trigger persistence.
type Config struct {
	path string `yaml:"-"`

	DeviceName        string `yaml:"device_name"`
	DownloadPath      string `yaml:"download_path"`
	Port              int    `yaml:"port"`             // 0 = random in 52100-52999; persisted once chosen
	AutoReceive       string `yaml:"auto_receive"`     // "ask" | "trusted" | "all"
	ConflictPolicy    string `yaml:"conflict_policy"`  // "skip" | "overwrite" | "keep_both"
	SharePortRange    string `yaml:"share_port_range"` // "52000-53000" or "auto"
	StealthMode       bool   `yaml:"stealth_mode"`
	NetworkInterface  string `yaml:"network_interface"` // "" = auto
	MaxActiveShares   int    `yaml:"max_active_shares"` // default 64, range 16-256
	Language          string `yaml:"language"`          // "zh" | "en" | "" = follow system
	NotifyReceive     bool   `yaml:"notify_receive"`
	NotifySent        bool   `yaml:"notify_sent"`
	NotifyShare       bool   `yaml:"notify_share"`
	RecordRetention   int    `yaml:"record_retention"` // 0 = unlimited; otherwise N most recent
	IPv6Preferred     bool   `yaml:"ipv6_preferred"`   // see PRD §3.1.8
	MobileDataAllowed bool   `yaml:"mobile_data_allowed"`

	mu sync.RWMutex
}

// Defaults returns a Config populated with MVP default values.
func Defaults() *Config {
	return &Config{
		DeviceName:        "", // filled from OS hostname at first run
		AutoReceive:       "ask",
		ConflictPolicy:    "skip",
		SharePortRange:    "auto",
		MaxActiveShares:   64,
		Language:          "",
		NotifyReceive:     true,
		NotifySent:        true,
		NotifyShare:       true,
		RecordRetention:   1000,
		IPv6Preferred:     true,
		MobileDataAllowed: false,
	}
}

// Load reads config.yaml from the data directory, applying defaults for
// any missing fields. If the file does not exist it is created with defaults.
func Load() (*Config, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config.yaml")

	cfg := Defaults()
	cfg.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read config: %w", err)
		}
		// First run: persist defaults + fill DeviceName.
		if cfg.DeviceName == "" {
			if host, err := os.Hostname(); err == nil {
				cfg.DeviceName = host
			} else {
				cfg.DeviceName = "Lanos-Device"
			}
		}
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("save initial config: %w", err)
		}
		return cfg, nil
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// Save writes the config back to disk atomically.
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// DataDir exposes the data directory path for other packages (store, etc.).
func (c *Config) DataDir() (string, error) {
	return dataDir()
}

// SetDeviceName updates the device name and persists.
func (c *Config) SetDeviceName(name string) error {
	c.mu.Lock()
	c.DeviceName = name
	c.mu.Unlock()
	return c.Save()
}

// dataDir mirrors identity.dataDir; duplicated here to keep config package
// standalone. Tests can override.
func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lanos"), nil
}
