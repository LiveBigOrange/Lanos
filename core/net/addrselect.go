package net

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// AddrPair is a destination/source address pair selected by RFC 6724.
// Destination is the dial address (IP:port for v4, [IP]:port for v6).
// Source is the best local source IP for this destination (may be "").
type AddrPair struct {
	Destination string
	Source      string
	IsV6        bool
}

// SelectAddresses applies RFC 6724 destination/source address selection to
// candidates advertised by a peer (`dsts`) and the local interface addresses
// (`sources`). Returns pairs sorted best-first. Destinations with no
// routable source (e.g. v6-only peer, v4-only local) are excluded. Callers
// should use the first non-empty result; an empty slice signals
// INCOMPATIBLE_IP_VERSION to the API layer.
//
// dsts may be destination IPs (link-local v6 may carry a zone id, e.g.
// "fe80::1%eth0") or "host:port" / "[host]:port" forms — the port is preserved
// in the returned Destination. sources are bare IPs.
func SelectAddresses(dsts []string, sources []string, port int) []AddrPair {
	parsedSrc := parseSources(sources)
	var entries []dstEntry
	for _, raw := range dsts {
		ip, zone, p, ok := splitHostPort(raw)
		if p == 0 {
			p = port
		}
		if !ok {
			continue
		}
		ipAddr := net.ParseIP(ip)
		if ipAddr == nil {
			continue
		}
		e := dstEntry{
			ip:    ipAddr,
			zone:  zone,
			isV6:  ipAddr.To4() == nil,
			scope: ipScope(ipAddr),
			prec:  ipPrecedence(ipAddr),
			port:  p,
		}
		e.src = pickSource(e, parsedSrc)
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return compareDst(entries[i], entries[j]) < 0
	})
	var out []AddrPair
	for _, e := range entries {
		// Drop unreachable destinations (no compatible source).
		if e.src == nil {
			continue
		}
		out = append(out, AddrPair{
			Destination: joinHostPort(e.ip, e.zone, e.port),
			Source:      e.src.IP.String(),
			IsV6:        e.isV6,
		})
	}
	return out
}

// SelectFirst returns the best dial address (host:port form) for the given
// candidates, or "" when no compatible pair exists. Convenience wrapper around
// SelectAddresses for callers that only want the top hit.
func SelectFirst(dsts []string, sources []string, port int) string {
	pairs := SelectAddresses(dsts, sources, port)
	if len(pairs) == 0 {
		return ""
	}
	return pairs[0].Destination
}

type dstEntry struct {
	ip    net.IP
	zone  string
	isV6  bool
	scope scope
	prec  int
	src   *sourceIP
	port  int
}

type sourceIP struct {
	IP    net.IP
	scope scope
	isV6  bool
}

func parseSources(sources []string) []sourceIP {
	var out []sourceIP
	for _, raw := range sources {
		ipStr := strings.TrimSpace(raw)
		if ipStr == "" {
			continue
		}
		// If source has zone, strip it for routing decisions.
		ipStr = stripZone(ipStr)
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsLoopback() || ip.IsInterfaceLocalMulticast() {
			// keep
		} else if ip.IsMulticast() {
			continue
		}
		out = append(out, sourceIP{
			IP:    ip,
			scope: ipScope(ip),
			isV6:  ip.To4() == nil,
		})
	}
	return out
}

type scope int

const (
	scopeLinkLocal scope = iota
	scopeLoopback
	scopeGlobal
	scopeOther
)

func ipScope(ip net.IP) scope {
	if ip.IsLoopback() {
		return scopeLoopback
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return scopeLinkLocal
	}
	if ip.IsGlobalUnicast() {
		return scopeGlobal
	}
	return scopeOther
}

// ipPrecedence returns the RFC 6724 §3.1 default-policy precedence for ip.
// Higher values = higher precedence (preferred).
func ipPrecedence(ip net.IP) int {
	v4 := ip.To4()
	if v4 != nil {
		// All IPv4 routed through ::ffff:0:0/96 (precedence 20).
		return 20
	}
	// IPv6 — match against each policy prefix.
	switch {
	case ip.IsLoopback(): // ::1/128
		return 50
	case ip.Equal(net.IPv6unspecified): // ::/128 unspecified
		return 40
	case isULA(ip): // fc00::/7
		return 7
	case isTeredo(ip): // 2001::/32
		return 5
	case is6to4(ip): // 2002::/16
		return 30
	default:
		return 40 // ::/0 default
	}
}

func isULA(ip net.IP) bool {
	if ip.To4() != nil {
		return false
	}
	return len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc
}

func is6to4(ip net.IP) bool {
	if ip.To4() != nil || len(ip) != net.IPv6len {
		return false
	}
	return ip[0] == 0x20 && ip[1] == 0x02
}

func isTeredo(ip net.IP) bool {
	if ip.To4() != nil || len(ip) != net.IPv6len {
		return false
	}
	return ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00
}

// pickSource implements RFC 6724 §5 simplified source selection for a dst.
// Returns nil if no compatible source exists (same version).
func pickSource(dst dstEntry, sources []sourceIP) *sourceIP {
	var best *sourceIP
	for i := range sources {
		s := &sources[i]
		if s.isV6 != dst.isV6 {
			continue // Rule 1/3: incompatible version.
		}
		if dst.scope == scopeLinkLocal && s.scope != scopeLinkLocal && s.scope != scopeLoopback {
			continue // Link-local dst needs link-local source.
		}
		if best == nil {
			best = s
			continue
		}
		if cmp := compareSource(dst, *best, *s); cmp < 0 {
			best = s
		}
	}
	return best
}

// compareSource returns <0 if candidate b is a better source than best for dst.
func compareSource(dst dstEntry, best sourceIP, candidate sourceIP) int {
	// Rule 2: prefer appropriate scope — same scope as dst wins.
	if candidate.scope == dst.scope && best.scope != dst.scope {
		return -1
	}
	if candidate.scope != dst.scope && best.scope == dst.scope {
		return 1
	}
	// Rule 6: prefer matching precedence-equivalent label — simplified to
	// preferring global scope when dst is global.
	if dst.scope == scopeGlobal {
		if candidate.scope == scopeGlobal && best.scope != scopeGlobal {
			return -1
		}
		if candidate.scope != scopeGlobal && best.scope == scopeGlobal {
			return 1
		}
	}
	// Rule 9: prefer longest matching prefix with dst.
	if c := commonPrefix(dst.ip, candidate.IP, dst.isV6); c != commonPrefix(dst.ip, best.IP, dst.isV6) {
		return c - commonPrefix(dst.ip, best.IP, dst.isV6)
	}
	return 0
}

func commonPrefix(a, b net.IP, v6 bool) int {
	aa := normalizeIP(a, v6)
	bb := normalizeIP(b, v6)
	if len(aa) != len(bb) {
		return 0
	}
	bits := 0
	for i := 0; i < len(aa); i++ {
		if aa[i] == bb[i] {
			bits += 8
			continue
		}
		x := aa[i] ^ bb[i]
		for mask := uint8(0x80); mask != 0; mask >>= 1 {
			if x&mask != 0 {
				return bits
			}
			bits++
		}
		return bits
	}
	return bits
}

func normalizeIP(ip net.IP, v6 bool) []byte {
	if v6 {
		if len(ip) == net.IPv4len {
			return ip.To16()
		}
		return ip
	}
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}

// compareDst implements RFC 6724 §6 destination ordering. Returns <0 if a
// should sort before b.
func compareDst(a, b dstEntry) int {
	// Reachable (has compatible source) beats unreachable regardless of
	// precedence — this lets happy-eyeballs callers skip dead addresses
	// without additional filtering.
	aReach := a.src != nil
	bReach := b.src != nil
	if aReach != bReach {
		if aReach {
			return -1
		}
		return 1
	}
	// Rule 2 of §6: prefer dst whose scope matches src scope.
	aScoped := a.src != nil && a.src.scope == a.scope
	bScoped := b.src != nil && b.src.scope == b.scope
	if aScoped != bScoped {
		if aScoped {
			return -1
		}
		return 1
	}
	// §6 Rule 8: prefer longest matching prefix with src.
	if a.src != nil && b.src != nil {
		aPrefix := commonPrefix(a.ip, a.src.IP, a.isV6)
		bPrefix := commonPrefix(b.ip, b.src.IP, b.isV6)
		if aPrefix != bPrefix {
			return bPrefix - aPrefix
		}
	}
	// §6 Rule 9: prefer higher precedence (table entry precedence value).
	if a.prec != b.prec {
		return b.prec - a.prec
	}
	// §6 Rule 10: otherwise preserve order (already stable by source order).
	if a.isV6 != b.isV6 {
		// Tiebreak: prefer v6 (happy-eyeballs default).
		if a.isV6 {
			return -1
		}
		return 1
	}
	return 0
}

// splitHostPort parses raw into host, zone, port. Handles "1.2.3.4:80",
// "[fe80::1%eth0]:80", "fe80::1%eth0" (no port), "1.2.3.4" (no port),
// "fe80::1". Returns ok=false on parse error.
func splitHostPort(raw string) (ip string, zone string, port int, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", 0, false
	}
	// Bracketed v6 with port: [host]:port
	if strings.HasPrefix(raw, "[") {
		end := strings.Index(raw, "]")
		if end < 0 {
			return "", "", 0, false
		}
		host := raw[1:end]
		rest := raw[end+1:]
		if rest == "" {
			return stripZone(host), "", 0, true
		}
		if !strings.HasPrefix(rest, ":") {
			return "", "", 0, false
		}
		p, ok := parsePort(rest[1:])
		if !ok {
			return "", "", 0, false
		}
		ipStr, zone := splitZone(host)
		return ipStr, zone, p, true
	}
	// Without brackets: "1.2.3.4:80", "host:port", or bare host.
	if host, ps, err := net.SplitHostPort(raw); err == nil {
		ipStr, zone := splitZone(host)
		p, ok := parsePort(ps)
		if !ok {
			return "", "", 0, false
		}
		return ipStr, zone, p, true
	}
	// Bare host (v4 or v6, possibly with zone, no port).
	ipStr, zone := splitZone(raw)
	return ipStr, zone, 0, true
}

func splitZone(s string) (ip string, zone string) {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

func stripZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}

func parsePort(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
		if n > 65535 {
			return 0, false
		}
	}
	return n, true
}

// joinHostPort formats a (ip, zone, port) tuple as a dial address.
func joinHostPort(ip net.IP, zone string, port int) string {
	if port == 0 {
		if ip.To4() != nil {
			return ip.String()
		}
		if zone != "" {
			return ip.String() + "%" + zone
		}
		return ip.String()
	}
	if ip.To4() != nil {
		return fmt.Sprintf("%s:%d", ip.String(), port)
	}
	if zone != "" {
		return fmt.Sprintf("[%s%%%s]:%d", ip.String(), zone, port)
	}
	return fmt.Sprintf("[%s]:%d", ip.String(), port)
}
