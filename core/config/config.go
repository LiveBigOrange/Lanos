// Package config loads and persists user settings to config.yaml.
// See PRD §3.5 and §5.3.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// Apply sets multiple fields from a map keyed by yaml tag and persists.
// Unknown or unexported fields are silently skipped. Type conversions
// follow JSON decoding conventions (float64->int for numeric fields).
func (c *Config) Apply(updates map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cfgType := reflect.TypeOf(c).Elem()
	cfgVal := reflect.ValueOf(c).Elem()

	for i := 0; i < cfgType.NumField(); i++ {
		f := cfgType.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		val, ok := updates[tag]
		if !ok {
			continue
		}
		field := cfgVal.Field(i)
		if !field.CanSet() {
			continue
		}
		rv := reflect.ValueOf(val)
		switch field.Kind() {
		case reflect.String:
			if rv.Kind() == reflect.String {
				field.SetString(rv.String())
			}
		case reflect.Bool:
			if rv.Kind() == reflect.Bool {
				field.SetBool(rv.Bool())
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				field.SetInt(rv.Int())
			case reflect.Float32, reflect.Float64:
				f := rv.Float()
				if math.IsNaN(f) || math.IsInf(f, 0) {
					continue
				}
				field.SetInt(int64(f))
			}
		case reflect.Float32, reflect.Float64:
			switch rv.Kind() {
			case reflect.Float32, reflect.Float64:
				f := rv.Float()
				if math.IsNaN(f) || math.IsInf(f, 0) {
					continue
				}
				field.SetFloat(f)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				field.SetFloat(float64(rv.Int()))
			}
		}
	}

	if err := c.validateLocked(); err != nil {
		return err
	}

	return c.Save()
}

func (c *Config) validateLocked() error {
	if c.DeviceName == "" {
		return fmt.Errorf("config: device_name must not be empty")
	}
	if c.Port != 0 && (c.Port < 1024 || c.Port > 65535) {
		return fmt.Errorf("config: port must be 0 or in range 1024-65535, got %d", c.Port)
	}
	if c.MaxActiveShares < 16 || c.MaxActiveShares > 256 {
		return fmt.Errorf("config: max_active_shares must be 16-256, got %d", c.MaxActiveShares)
	}
	switch c.AutoReceive {
	case "", "ask", "trusted", "all":
	default:
		return fmt.Errorf("config: auto_receive must be ask/trusted/all, got %q", c.AutoReceive)
	}
	return nil
}

// SetDeviceName updates the device name and persists.
func (c *Config) SetDeviceName(name string) error {
	c.mu.Lock()
	c.DeviceName = name
	if err := c.validateLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
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
