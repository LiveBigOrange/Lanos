package discovery

import (
	"net/url"
	"runtime"
	"strings"
	"testing"

	"github.com/lanos/lanos/core/config"
	"github.com/lanos/lanos/core/identity"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	c := config.Defaults()
	c.DeviceName = "TestDevice"
	c.Port = 52150
	return c
}

func testIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir+"/.config")
	t.Setenv("APPDATA", dir)
	ident, err := identity.LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	return ident
}

func TestBuildTXT_AllFields(t *testing.T) {
	cfg := testConfig(t)
	ident := testIdentity(t)
	txt, err := BuildTXT(cfg, ident, "46")
	if err != nil {
		t.Fatalf("BuildTXT: %v", err)
	}
	wantFields := map[string]string{
		"txt-ver":     "1",
		"proto":       "lanos/1.0",
		"platform":    runtime.GOOS,
		"port":        "52150",
		"pk-hash":     ident.PubHash,
		"device-name": url.QueryEscape("TestDevice"),
		"ip-ver":      "46",
	}
	got := map[string]string{}
	for _, line := range txt {
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			t.Fatalf("malformed TXT line %q", line)
		}
		got[line[:eq]] = line[eq+1:]
	}
	for k, w := range wantFields {
		if g := got[k]; g != w {
			t.Errorf("TXT[%s] = %q, want %q", k, g, w)
		}
	}
	if len(got) != len(wantFields) {
		t.Errorf("TXT has %d fields, want %d (extra=%v)", len(got), len(wantFields), extraKeys(got, wantFields))
	}
}

func TestBuildTXT_InvalidIPVer(t *testing.T) {
	cfg := testConfig(t)
	ident := testIdentity(t)
	if _, err := BuildTXT(cfg, ident, "5"); err == nil {
		t.Fatal("expected error for ip-ver=5")
	}
	if _, err := BuildTXT(cfg, ident, ""); err == nil {
		t.Fatal("expected error for empty ip-ver")
	}
}

func TestBuildTXT_InvalidPort(t *testing.T) {
	cfg := testConfig(t)
	cfg.Port = 0
	ident := testIdentity(t)
	if _, err := BuildTXT(cfg, ident, "4"); err == nil {
		t.Fatal("expected error for port=0")
	}
	cfg.Port = 70000
	if _, err := BuildTXT(cfg, ident, "4"); err == nil {
		t.Fatal("expected error for port=70000")
	}
}

func TestBuildTXT_NilArgs(t *testing.T) {
	if _, err := BuildTXT(nil, nil, "4"); err == nil {
		t.Fatal("expected error for nil args")
	}
}

func TestParseTXT_RoundTrip(t *testing.T) {
	cfg := testConfig(t)
	ident := testIdentity(t)
	cfg.DeviceName = "Möbius 设备" // non-ASCII to exercise URL encoding
	txt, err := BuildTXT(cfg, ident, "46")
	if err != nil {
		t.Fatalf("BuildTXT: %v", err)
	}
	rec, err := ParseTXT(txt)
	if err != nil {
		t.Fatalf("ParseTXT: %v", err)
	}
	if rec.TxtVersion != "1" {
		t.Errorf("TxtVersion = %q", rec.TxtVersion)
	}
	if rec.Proto != "lanos/1.0" {
		t.Errorf("Proto = %q", rec.Proto)
	}
	if rec.Platform != runtime.GOOS {
		t.Errorf("Platform = %q, want %q", rec.Platform, runtime.GOOS)
	}
	if rec.Port != cfg.Port {
		t.Errorf("Port = %d, want %d", rec.Port, cfg.Port)
	}
	if rec.PubHash != ident.PubHash {
		t.Errorf("PubHash = %q, want %q", rec.PubHash, ident.PubHash)
	}
	if rec.DeviceName != cfg.DeviceName {
		t.Errorf("DeviceName = %q, want %q", rec.DeviceName, cfg.DeviceName)
	}
	if rec.IPVersion != "46" {
		t.Errorf("IPVersion = %q", rec.IPVersion)
	}
}

func TestParseTXT_MissingRequired(t *testing.T) {
	cases := [][]string{
		{"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=52150", "pk-hash=abcdef0123456789abcdef0123456789"}, // missing ip-ver
		{"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=52150", "ip-ver=46"},                                // missing pk-hash
		{"proto=lanos/1.0", "platform=linux", "port=52150", "pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=46"}, // missing txt-ver
	}
	for i, txt := range cases {
		if _, err := ParseTXT(txt); err == nil {
			t.Errorf("case %d: expected error for missing field", i)
		}
	}
}

func TestParseTXT_InvalidValues(t *testing.T) {
	cases := []struct {
		name string
		txt  []string
	}{
		{"bad txt-ver", []string{"txt-ver=2", "proto=lanos/1.0", "platform=linux", "port=52150", "pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=46"}},
		{"bad proto", []string{"txt-ver=1", "proto=foo/1.0", "platform=linux", "port=52150", "pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=46"}},
		{"bad ip-ver", []string{"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=52150", "pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=8"}},
		{"short pk-hash", []string{"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=52150", "pk-hash=abc", "ip-ver=46"}},
		{"bad port", []string{"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=notanumber", "pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=46"}},
		{"port out of range", []string{"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=99999", "pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=46"}},
		{"bad device-name escape", []string{"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=52150", "pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=46", "device-name=%ZZ"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseTXT(tc.txt); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestParseTXT_IgnoresUnknownFields(t *testing.T) {
	txt := []string{
		"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=52150",
		"pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=46",
		"unknown=foo", "extra=bar",
	}
	if _, err := ParseTXT(txt); err != nil {
		t.Fatalf("ParseTXT with unknown fields: %v", err)
	}
}

func TestParseTXT_EmptyDeviceName(t *testing.T) {
	// device-name is optional per the parser; legacy peers may omit it.
	txt := []string{
		"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=52150",
		"pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=46",
	}
	rec, err := ParseTXT(txt)
	if err != nil {
		t.Fatalf("ParseTXT: %v", err)
	}
	if rec.DeviceName != "" {
		t.Errorf("DeviceName = %q, want empty", rec.DeviceName)
	}
}

func TestParseTXT_MalformedLines(t *testing.T) {
	// Lines without '=' or with empty key should be skipped, not fatal.
	txt := []string{
		"txt-ver=1", "proto=lanos/1.0", "platform=linux", "port=52150",
		"pk-hash=abcdef0123456789abcdef0123456789", "ip-ver=46",
		"noequals", "=novalue",
	}
	if _, err := ParseTXT(txt); err != nil {
		t.Fatalf("ParseTXT with malformed lines: %v", err)
	}
}

func extraKeys(got, want map[string]string) []string {
	var extra []string
	for k := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	return extra
}
