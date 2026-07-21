package net

import (
	"strings"
	"testing"
)

func TestParseConnectURIValidDualStack(t *testing.T) {
	raw := "lanos://connect?ip=192.168.1.50&ip6=fd00::1&port=52110&pk-hash=0123456789abcdef0123456789abcdef&device-name=alice%20laptop"
	c, err := ParseConnectURI(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.IP != "192.168.1.50" {
		t.Errorf("IP = %q, want 192.168.1.50", c.IP)
	}
	if c.IP6 != "fd00::1" {
		t.Errorf("IP6 = %q, want fd00::1", c.IP6)
	}
	if c.Port != 52110 {
		t.Errorf("Port = %d, want 52110", c.Port)
	}
	if c.PKHash != "0123456789abcdef0123456789abcdef" {
		t.Errorf("PKHash = %q", c.PKHash)
	}
	if c.DeviceName != "alice laptop" {
		t.Errorf("DeviceName = %q, want \"alice laptop\"", c.DeviceName)
	}
	if c.Raw != raw {
		t.Errorf("Raw not preserved")
	}
}

func TestParseConnectURIIPv6Only(t *testing.T) {
	raw := "lanos://connect?ip6=2001:db8::1&port=9999&pk-hash=abcdef0123456789abcdef0123456789&device-name=b"
	c, err := ParseConnectURI(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.IP != "" {
		t.Errorf("IP should be empty for v6-only URI, got %q", c.IP)
	}
	if c.IP6 != "2001:db8::1" {
		t.Errorf("IP6 = %q", c.IP6)
	}
}

func TestParseConnectURILinkLocalRequiresZone(t *testing.T) {
	good := "lanos://connect?ip6=fe80::1%25wlan0&port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
	if _, err := ParseConnectURI(good); err != nil {
		t.Errorf("expected zone-tagged link-local to parse, got: %v", err)
	}
	bad := "lanos://connect?ip6=fe80::1&port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
	_, err := ParseConnectURI(bad)
	if err == nil || !strings.Contains(err.Error(), "zone id") {
		t.Fatalf("expected zone-id error, got: %v", err)
	}
}

func TestParseConnectURIMissingIPFamily(t *testing.T) {
	raw := "lanos://connect?port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
	_, err := ParseConnectURI(raw)
	if err == nil || !strings.Contains(err.Error(), "at least one of ip / ip6") {
		t.Fatalf("expected missing-IP error, got: %v", err)
	}
}

func TestParseConnectURIMissingRequired(t *testing.T) {
	cases := map[string]string{
		"missing port":        "lanos://connect?ip=192.0.2.1&pk-hash=0123456789abcdef0123456789abcdef&device-name=x",
		"missing pk-hash":     "lanos://connect?ip=192.0.2.1&port=52100&device-name=x",
		"missing device-name": "lanos://connect?ip=192.0.2.1&port=52100&pk-hash=0123456789abcdef0123456789abcdef",
	}
	for name, raw := range cases {
		_, err := ParseConnectURI(raw)
		if err == nil {
			t.Errorf("%s: expected error, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "required") {
			t.Errorf("%s: expected 'required' in error, got: %v", name, err)
		}
	}
}

func TestParseConnectURIBadSchemePath(t *testing.T) {
	cases := map[string]string{
		"wrong scheme": "https://connect?ip=1.1.1.1&port=1&pk-hash=0123456789abcdef0123456789abcdef&device-name=x",
		"wrong path":   "lanos://discover?ip=1.1.1.1&port=1&pk-hash=0123456789abcdef0123456789abcdef&device-name=x",
		"empty":        "",
	}
	for name, raw := range cases {
		_, err := ParseConnectURI(raw)
		if err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestParseConnectURIBadPKHash(t *testing.T) {
	cases := map[string]string{
		"too short":     "0123456789abcdef0123456789abcd",
		"too long":      "0123456789abcdef0123456789abcdef00",
		"upper case":    "0123456789ABCDEF0123456789abcdef",
		"non-hex chars": "0123456789abcdez0123456789abcdef",
	}
	for name, pk := range cases {
		raw := "lanos://connect?ip=1.1.1.1&port=1&pk-hash=" + pk + "&device-name=x"
		_, err := ParseConnectURI(raw)
		if err == nil {
			t.Errorf("%s: expected error, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "pk-hash") {
			t.Errorf("%s: expected pk-hash in error, got: %v", name, err)
		}
	}
}

func TestParseConnectURIPortOutOfRange(t *testing.T) {
	for _, p := range []string{"0", "65536", "-1", "abc"} {
		raw := "lanos://connect?ip=1.1.1.1&port=" + p + "&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
		_, err := ParseConnectURI(raw)
		if err == nil || !strings.Contains(err.Error(), "port") {
			t.Errorf("port=%q: expected port error, got: %v", p, err)
		}
	}
}

func TestParseConnectURIBadIPv4(t *testing.T) {
	raw := "lanos://connect?ip=999.1.1.1&port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
	_, err := ParseConnectURI(raw)
	if err == nil || !strings.Contains(err.Error(), "ip not a valid IPv4") {
		t.Fatalf("expected IPv4 parse error, got: %v", err)
	}
}

func TestParseConnectURIBadIPv6(t *testing.T) {
	raw := "lanos://connect?ip6=notv6&port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
	_, err := ParseConnectURI(raw)
	if err == nil || !strings.Contains(err.Error(), "ip6 not a valid IPv6") {
		t.Fatalf("expected IPv6 parse error, got: %v", err)
	}
}

func TestParseConnectURIRejectsIP6AsIP(t *testing.T) {
	raw := "lanos://connect?ip=::1&port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
	_, err := ParseConnectURI(raw)
	if err == nil || !strings.Contains(err.Error(), "ip not a valid IPv4") {
		t.Fatalf("expected IPv4-only-mismatch error for ::1 in ip, got: %v", err)
	}
}

func TestParseConnectURIRejectsV4InIP6(t *testing.T) {
	raw := "lanos://connect?ip6=192.0.2.1&port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
	_, err := ParseConnectURI(raw)
	if err == nil || !strings.Contains(err.Error(), "ip6 not a valid IPv6") {
		t.Fatalf("expected IPv6-only-mismatch error for 192.0.2.1 in ip6, got: %v", err)
	}
}

func TestParseConnectURIUnknownParameter(t *testing.T) {
	raw := "lanos://connect?ip=1.1.1.1&port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=x&extra=bad"
	_, err := ParseConnectURI(raw)
	if err == nil || !strings.Contains(err.Error(), "unknown parameter") {
		t.Fatalf("expected unknown-parameter error, got: %v", err)
	}
}

func TestParseConnectURIDuplicateParameter(t *testing.T) {
	raw := "lanos://connect?ip=1.1.1.1&ip=2.2.2.2&port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
	_, err := ParseConnectURI(raw)
	if err == nil || !strings.Contains(err.Error(), "duplicate parameter") {
		t.Fatalf("expected duplicate-parameter error, got: %v", err)
	}
}

func TestConnectURIDests(t *testing.T) {
	c := &ConnectURI{IP: "192.0.2.1", IP6: "fd00::1"}
	dests := c.Dests()
	if len(dests) != 2 || dests[0] != "192.0.2.1" || dests[1] != "fd00::1" {
		t.Fatalf("Dests() = %v, want [192.0.2.1, fd00::1]", dests)
	}
	c2 := &ConnectURI{IP6: "2001:db8::1"}
	if d := c2.Dests(); len(d) != 1 || d[0] != "2001:db8::1" {
		t.Fatalf("Dests() = %v", d)
	}
}

func TestConnectURIRoundTrip(t *testing.T) {
	original := "lanos://connect?ip=192.168.1.50&ip6=fd00::1&port=52110&pk-hash=0123456789abcdef0123456789abcdef&device-name=alice%20laptop"
	c, err := ParseConnectURI(original)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	round := c.String()
	c2, err := ParseConnectURI(round)
	if err != nil {
		t.Fatalf("reparse: %v\nround=%s", err, round)
	}
	if c2.IP != c.IP || c2.IP6 != c.IP6 || c2.Port != c.Port || c2.PKHash != c.PKHash || c2.DeviceName != c.DeviceName {
		t.Errorf("round-trip mismatch:\n c=%+v\nc2=%+v", c, c2)
	}
}

func TestConnectURIRoundTripLinkLocalZone(t *testing.T) {
	original := "lanos://connect?ip6=fe80::1%25wlan0&port=52110&pk-hash=0123456789abcdef0123456789abcdef&device-name=bob%27s%20phone"
	c, err := ParseConnectURI(original)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.IP6 != "fe80::1%wlan0" {
		t.Fatalf("IP6 after parse = %q, want fe80::1%%wlan0", c.IP6)
	}
	round := c.String()
	c2, err := ParseConnectURI(round)
	if err != nil {
		t.Fatalf("reparse: %v\nround=%s", err, round)
	}
	if c2.IP6 != c.IP6 {
		t.Errorf("IP6 round-trip mismatch: %q vs %q", c2.IP6, c.IP6)
	}
}

func TestConnectURIDestsFeedIntoSelectAddresses(t *testing.T) {
	// Integration: a parsed multi-family URI's Dests() should yield a v6-first
	// selection when a v6 source is available.
	raw := "lanos://connect?ip=192.168.1.50&ip6=2001:db8::1&port=52110&pk-hash=0123456789abcdef0123456789abcdef&device-name=x"
	c, err := ParseConnectURI(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	srcs := []string{"192.168.1.5", "2001:db8::5"}
	pairs := SelectAddresses(c.Dests(), srcs, c.Port)
	if len(pairs) == 0 {
		t.Fatalf("expected non-empty selection")
	}
	if !pairs[0].IsV6 {
		t.Errorf("expected v6 first (happy-eyeballs), got v4: %+v", pairs[0])
	}
	if pairs[0].Destination != "[2001:db8::1]:52110" {
		t.Errorf("expected [2001:db8::1]:52110 first, got %q", pairs[0].Destination)
	}
}
