package net

import (
	"encoding/hex"
	"errors"
	"fmt"
	stdnet "net"
	"net/url"
	"strconv"
	"strings"
)

// ConnectURI represents a parsed lanos://connect URI. See docs/PROTOCOL.md §2.
//
// Format: lanos://connect?ip=<v4>&ip6=<v6>&port=<int>&pk-hash=<hex16>&device-name=<urlenc>
//
// Rules:
//   - Exactly one occurrence of each parameter; duplicates are an error.
//   - At least one of ip / ip6 must be present.
//   - port is required and 1..65535.
//   - pk-hash is required, must be 32 lowercase hex characters (16 bytes).
//   - device-name is required, must URL-decode to a non-empty string.
//   - ip6 link-local addresses (fe80::/10) must carry a zone id, e.g.
//     "fe80::1%wlan0"; non-link-local v6 may omit the zone.
type ConnectURI struct {
	IP         string
	IP6        string
	Port       int
	PKHash     string
	DeviceName string
	Raw        string
}

// ParseConnectURI parses a lanos://connect URI. Returns an error describing
// the first violation of the scheme contract.
func ParseConnectURI(raw string) (*ConnectURI, error) {
	if !strings.HasPrefix(raw, "lanos://") {
		return nil, errors.New("lanos: uri: missing lanos:// scheme")
	}
	// Strip scheme; remainder is "connect?query" or "connect".
	rest := raw[len("lanos://"):]
	path, query, _ := strings.Cut(rest, "?")
	if path != "connect" {
		return nil, fmt.Errorf("lanos: uri: unexpected path %q (want \"connect\")", path)
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return nil, fmt.Errorf("lanos: uri: bad query: %w", err)
	}
	c := &ConnectURI{Raw: raw}

	for key := range values {
		switch key {
		case "ip", "ip6", "port", "pk-hash", "device-name":
		default:
			return nil, fmt.Errorf("lanos: uri: unknown parameter %q", key)
		}
	}

	if v, ok, err := single(values, "ip"); err != nil {
		return nil, err
	} else if ok {
		if v == "" {
			return nil, errors.New("lanos: uri: empty ip")
		}
		if stdnet.ParseIP(v) == nil || stdnet.ParseIP(v).To4() == nil {
			return nil, fmt.Errorf("lanos: uri: ip not a valid IPv4 literal: %q", v)
		}
		c.IP = v
	}
	if v, ok, err := single(values, "ip6"); err != nil {
		return nil, err
	} else if ok {
		if v == "" {
			return nil, errors.New("lanos: uri: empty ip6")
		}
		ip, zone := splitZone(v)
		if stdnet.ParseIP(ip) == nil || stdnet.ParseIP(ip).To4() != nil {
			return nil, fmt.Errorf("lanos: uri: ip6 not a valid IPv6 literal: %q", v)
		}
		if isLinkLocalV6(ip) && zone == "" {
			return nil, fmt.Errorf("lanos: uri: ip6 link-local requires zone id: %q", v)
		}
		c.IP6 = v
	}

	if c.IP == "" && c.IP6 == "" {
		return nil, errors.New("lanos: uri: at least one of ip / ip6 required")
	}

	if v, ok, err := single(values, "port"); err != nil {
		return nil, err
	} else if ok {
		p, perr := strconv.Atoi(v)
		if perr != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("lanos: uri: port out of range 1..65535: %q", v)
		}
		c.Port = p
	} else {
		return nil, errors.New("lanos: uri: port is required")
	}

	if v, ok, err := single(values, "pk-hash"); err != nil {
		return nil, err
	} else if ok {
		if len(v) != 32 || !isLowerHex(v) {
			return nil, fmt.Errorf("lanos: uri: pk-hash must be 32 lowercase hex chars: %q", v)
		}
		if _, herr := hex.DecodeString(v); herr != nil {
			return nil, fmt.Errorf("lanos: uri: pk-hash not valid hex: %w", herr)
		}
		c.PKHash = v
	} else {
		return nil, errors.New("lanos: uri: pk-hash is required")
	}

	if v, ok, err := single(values, "device-name"); err != nil {
		return nil, err
	} else if ok {
		decoded, derr := url.QueryUnescape(v)
		if derr != nil {
			return nil, fmt.Errorf("lanos: uri: device-name not url-encoded: %w", derr)
		}
		if decoded == "" {
			return nil, errors.New("lanos: uri: device-name is empty")
		}
		c.DeviceName = decoded
	} else {
		return nil, errors.New("lanos: uri: device-name is required")
	}

	return c, nil
}

// Dests returns the candidate destination addresses for this URI, suitable
// for direct feed into SelectAddresses. Strips zone id from link-local v6
// only when SelectAddresses re-parses it — kept on the URI value as-is so the
// dial string retains the zone.
func (c *ConnectURI) Dests() []string {
	var out []string
	if c.IP != "" {
		out = append(out, c.IP)
	}
	if c.IP6 != "" {
		out = append(out, c.IP6)
	}
	return out
}

// String reformats the URI canonically (ordered params, url-encoded
// device-name). Useful for QR generation on the sender side.
func (c *ConnectURI) String() string {
	var b strings.Builder
	b.WriteString("lanos://connect?")
	if c.IP != "" {
		b.WriteString("ip=")
		b.WriteString(c.IP)
		b.WriteString("&")
	}
	if c.IP6 != "" {
		b.WriteString("ip6=")
		b.WriteString(escapePercent(c.IP6))
		b.WriteString("&")
	}
	b.WriteString("port=")
	b.WriteString(strconv.Itoa(c.Port))
	b.WriteString("&pk-hash=")
	b.WriteString(c.PKHash)
	b.WriteString("&device-name=")
	b.WriteString(url.QueryEscape(c.DeviceName))
	return b.String()
}

// single returns the (only) value for a key, or false if absent. Multiple
// occurrences are an error.
func single(v url.Values, key string) (string, bool, error) {
	vs, ok := v[key]
	if !ok || len(vs) == 0 {
		return "", false, nil
	}
	if len(vs) > 1 {
		return "", false, fmt.Errorf("lanos: uri: duplicate parameter %q", key)
	}
	return vs[0], true, nil
}

func isLowerHex(s string) bool {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

// escapePercent url-encodes only the "%" character, used for ip6 zone ids
// (fe80::1%wlan0 → fe80::1%25wlan0). Other IPv6 chars are URL-safe.
func escapePercent(s string) string {
	return strings.ReplaceAll(s, "%", "%25")
}

func isLinkLocalV6(s string) bool {
	ip := stdnet.ParseIP(s)
	if ip == nil || ip.To4() != nil {
		return false
	}
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
