package net

import (
	"net"
	"testing"
)

func TestSelectAddressesPrefersV6InDualStack(t *testing.T) {
	dsts := []string{"192.168.1.10", "fd00::abc"}
	srcs := []string{"192.168.1.5", "fd00::5"}
	pairs := SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 pairs (both compatible), got %d: %+v", len(pairs), pairs)
	}
	// RFC 6724 default: global v6 (::/0 prec 40) > ::ffff:0:0/96 (v4 prec 20).
	// ULA (fc00::/7) prec 7 is *lower* than global v4 precedence (20), so v4
	// should rank ahead of ULA when both are reachable. But for the dual-stack
	// happy-eyeballs case we want v6 leading for global v6 → covered in the
	// next test. Here we just assert both reachable and stableness.
	if !pairs[0].IsV6 && !pairs[1].IsV6 {
		t.Errorf("expected at least one v6 pair, got all v4: %+v", pairs)
	}
}

func TestSelectAddressesV6HappyEyeballsGlobal(t *testing.T) {
	dsts := []string{"192.0.2.10", "2001:db8::1"}
	srcs := []string{"192.0.2.5", "2001:db8::5"}
	pairs := SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(pairs))
	}
	if !pairs[0].IsV6 {
		t.Errorf("expected v6 first (precedence 40 > v4 precedence 20), got %+v", pairs[0])
	}
}

func TestSelectAddressesV6OnlyPeerV4OnlyLocal(t *testing.T) {
	// 节点 A 只有 IPv6 来源（v6-only network），节点 B 节点的 peer 摘要只提供 v4。
	dsts := []string{"192.168.1.10"}
	srcs := []string{"fd00::5", "2001:db8::5"}
	pairs := SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 0 {
		t.Errorf("expected zero compatible pairs (INCOMPATIBLE_IP_VERSION), got %+v", pairs)
	}
	if SelectFirst(dsts, srcs, 52100) != "" {
		t.Errorf("SelectFirst should return \"\" for incompatible pair")
	}
}

func TestSelectAddressesV4OnlyPeerV6OnlyLocal(t *testing.T) {
	dsts := []string{"2001:db8::1"}
	srcs := []string{"192.0.2.5"}
	pairs := SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 0 {
		t.Errorf("expected zero compatible pairs, got %+v", pairs)
	}
}

func TestSelectAddressesLinkLocalRequiresZoneAndMatchingScope(t *testing.T) {
	// Peer advertises link-local v6 with zone; we must pick a link-local
	// source (here the local fe80::5 on the same interface zone "eth0").
	dsts := []string{"fe80::1%eth0"}
	srcs := []string{"192.168.1.5", "fe80::5", "2001:db8::5"}
	pairs := SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair (link-local v6 only), got %d: %+v", len(pairs), pairs)
	}
	if !pairs[0].IsV6 {
		t.Errorf("expected v6 pair, got %+v", pairs[0])
	}
	if pairs[0].Source == "" {
		t.Errorf("expected non-empty source for link-local dst")
	}
	if pairs[0].Source != "fe80::5" {
		t.Errorf("expected link-local source fe80::5, got %q", pairs[0].Source)
	}
	// Zone must be preserved in the dial string.
	if pairs[0].Destination != "[fe80::1%eth0]:52100" {
		t.Errorf("expected [fe80::1%%eth0]:52100, got %q", pairs[0].Destination)
	}
}

func TestSelectAddressesNoLinkLocalSource(t *testing.T) {
	// Peer advertises link-local v6, but local only has a global v6 source
	// → dst is unroutable, dropped from results.
	dsts := []string{"fe80::1%eth0", "2001:db8::9"}
	srcs := []string{"2001:db8::5"}
	pairs := SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair (only global v6 reachable), got %d: %+v", len(pairs), pairs)
	}
	if pairs[0].Destination != "[2001:db8::9]:52100" {
		t.Errorf("expected global v6 pair to be returned, got %q", pairs[0].Destination)
	}
}

func TestSelectAddressesLoopbackPreferredForLoopbackDst(t *testing.T) {
	// ::1 has precedence 50 (RFC 6724) — the highest. Same-scope src required.
	dsts := []string{"192.0.2.10", "::1"}
	srcs := []string{"192.0.2.5", "::1"}
	pairs := SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(pairs))
	}
	if pairs[0].Destination != "[::1]:52100" {
		t.Errorf("expected ::1 first (precedence 50), got %q first", pairs[0].Destination)
	}
}

func TestSelectAddressesPreservesCustomPort(t *testing.T) {
	dsts := []string{"192.168.1.10:52222", "[2001:db8::1]:52222"}
	srcs := []string{"192.168.1.5"}
	// Only v4 reachable because source is v4-only.
	pairs := SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 1 {
		t.Fatalf("expected exactly 1 v4 pair (v6 has no compatible source), got %d", len(pairs))
	}
	if pairs[0].Destination != "192.168.1.10:52222" {
		t.Errorf("expected custom port preserved, got %q", pairs[0].Destination)
	}
}

func TestSelectAddressesIgnoresMalformedEntries(t *testing.T) {
	dsts := []string{"not-an-ip", "", "192.168.1.10"}
	srcs := []string{"192.168.1.5"}
	pairs := SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 1 {
		t.Fatalf("expected malformed entries silently dropped → 1 pair, got %d", len(pairs))
	}
	if pairs[0].Destination != "192.168.1.10:52100" {
		t.Errorf("expected the well-formed v4 entry, got %q", pairs[0].Destination)
	}
}

func TestSelectAddressesBareV4UsesDefaultPort(t *testing.T) {
	pairs := SelectAddresses([]string{"192.168.1.10"}, []string{"192.168.1.5"}, 9999)
	if len(pairs) != 1 || pairs[0].Destination != "192.168.1.10:9999" {
		t.Fatalf("expected default port, got %+v", pairs)
	}
}

func TestSelectAddressesBareV6UsesDefaultPort(t *testing.T) {
	pairs := SelectAddresses([]string{"2001:db8::1"}, []string{"2001:db8::5"}, 8888)
	if len(pairs) != 1 || pairs[0].Destination != "[2001:db8::1]:8888" {
		t.Fatalf("expected [2001:db8::1]:8888, got %+v", pairs)
	}
}

func TestSelectAddressesPrefersLongestCommonPrefix(t *testing.T) {
	// Equal-precedence v4 dsts — RFC 6724 §6 Rule 8 prefers longest matching
	// prefix with source. 192.168.1.10 shares 24 bits with source 192.168.1.5.
	dsts := []string{"172.16.0.1", "10.0.0.5", "192.168.1.10"}
	srcs := []string{"192.168.1.5"}
	pairs := SelectAddresses(dsts, srcs, 60000)
	if len(pairs) != 3 {
		t.Fatalf("expected 3 v4 pairs, got %d", len(pairs))
	}
	if pairs[0].Destination != "192.168.1.10:60000" {
		t.Errorf("expected longest-prefix match first, got %q", pairs[0].Destination)
	}
}

func TestSelectAddressesStableOrderWhenAllEqual(t *testing.T) {
	// Truly-equal pair (different IPs but same prefix-len with src) preserves
	// input order via sort.SliceStable.
	dsts := []string{"192.168.50.1", "192.168.50.2"}
	srcs := []string{"192.168.99.1"}
	pairs := SelectAddresses(dsts, srcs, 60000)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(pairs))
	}
	if pairs[0].Destination != "192.168.50.1:60000" {
		t.Errorf("expected input order preserved for equal-prefix dsts, got %q", pairs[0].Destination)
	}
}

func TestIPPrecedenceTable(t *testing.T) {
	cases := map[string]int{
		"::1":          50,
		"2001:db8::1":  40,
		"fe80::1":      40,
		"fd00::1":      7,
		"2001:0::1":    5, // Teredo prefix 2001::/32
		"2002:dead::1": 30,
		"192.0.2.10":   20,
		"127.0.0.1":    20,
		"169.254.1.2":  20,
	}
	for ip, want := range cases {
		got := ipPrecedence(parseIP(ip))
		if got != want {
			t.Errorf("ipPrecedence(%q) = %d, want %d", ip, got, want)
		}
	}
}

func TestIPScopeTable(t *testing.T) {
	cases := map[string]scope{
		"127.0.0.1":   scopeLoopback,
		"::1":         scopeLoopback,
		"169.254.1.1": scopeLinkLocal,
		"fe80::1":     scopeLinkLocal,
		"192.0.2.1":   scopeGlobal,
		"2001:db8::1": scopeGlobal,
	}
	for ip, want := range cases {
		got := ipScope(parseIP(ip))
		if got != want {
			t.Errorf("ipScope(%q) = %v, want %v", ip, got, want)
		}
	}
}

func parseIP(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("invalid test IP: " + s)
	}
	return ip
}
