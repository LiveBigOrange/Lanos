package discovery

import (
	"fmt"
	"net/url"
	"runtime"
	"strconv"
	"strings"

	"github.com/lanos/lanos/core/config"
	"github.com/lanos/lanos/core/identity"
)

// TXT record schema version. Bumped when the TXT field set changes.
// See PRD §3.1.1.
const txtVersion = "1"

// ServiceType is the DNS-SD service type used by Lanos.
const ServiceType = "_lanos._tcp"
const ServiceDomain = "local."

// Protocol version advertised in the proto= TXT field.
const ProtoVersion = "lanos/1.0"

// TXTRecord is the parsed form of the mDNS TXT records. See PRD §3.1.1.
type TXTRecord struct {
	TxtVersion string // txt-ver
	Proto      string // proto
	Platform   string // platform (linux/darwin/windows/android/ios)
	Port       int    // port
	PubHash    string // pk-hash (16 hex chars)
	DeviceName string // device-name (URL-decoded)
	IPVersion  string // ip-ver (4 | 6 | 46)
}

// BuildTXT renders the TXT records for the local service announcement.
// ipVer must be one of "4", "6", "46".
func BuildTXT(cfg *config.Config, ident *identity.Identity, ipVer string) ([]string, error) {
	if cfg == nil || ident == nil {
		return nil, fmt.Errorf("cfg and ident must not be nil")
	}
	if ipVer != "4" && ipVer != "6" && ipVer != "46" {
		return nil, fmt.Errorf("invalid ipVer %q (want 4|6|46)", ipVer)
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid port %d", cfg.Port)
	}
	if ident.PubHash == "" {
		return nil, fmt.Errorf("identity has empty PubHash")
	}
	return []string{
		"txt-ver=" + txtVersion,
		"proto=" + ProtoVersion,
		"platform=" + runtime.GOOS,
		"port=" + strconv.Itoa(cfg.Port),
		"pk-hash=" + ident.PubHash,
		"device-name=" + url.QueryEscape(cfg.DeviceName),
		"ip-ver=" + ipVer,
	}, nil
}

// ParseTXT parses the TXT records from a ServiceEntry. Unknown fields are
// ignored. Missing required fields produce an error.
func ParseTXT(text []string) (TXTRecord, error) {
	var rec TXTRecord
	seen := map[string]bool{}
	for _, line := range text {
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := line[:eq]
		val := line[eq+1:]
		seen[key] = true
		switch key {
		case "txt-ver":
			rec.TxtVersion = val
		case "proto":
			rec.Proto = val
		case "platform":
			rec.Platform = val
		case "port":
			p, err := strconv.Atoi(val)
			if err != nil {
				return rec, fmt.Errorf("invalid port %q: %w", val, err)
			}
			rec.Port = p
		case "pk-hash":
			rec.PubHash = val
		case "device-name":
			name, err := url.QueryUnescape(val)
			if err != nil {
				return rec, fmt.Errorf("invalid device-name %q: %w", val, err)
			}
			rec.DeviceName = name
		case "ip-ver":
			rec.IPVersion = val
		}
	}
	// Required fields per PRD §3.1.1. device-name may be absent in legacy peers.
	for _, k := range []string{"txt-ver", "proto", "platform", "port", "pk-hash", "ip-ver"} {
		if !seen[k] {
			return rec, fmt.Errorf("missing required TXT field %q", k)
		}
	}
	if rec.TxtVersion != txtVersion {
		return rec, fmt.Errorf("unsupported txt-ver %q (want %q)", rec.TxtVersion, txtVersion)
	}
	if !strings.HasPrefix(rec.Proto, "lanos/") {
		return rec, fmt.Errorf("unsupported proto %q", rec.Proto)
	}
	if rec.IPVersion != "4" && rec.IPVersion != "6" && rec.IPVersion != "46" {
		return rec, fmt.Errorf("invalid ip-ver %q", rec.IPVersion)
	}
	if len(rec.PubHash) != 32 {
		return rec, fmt.Errorf("invalid pk-hash length %d (want 32 hex chars = 16 bytes)", len(rec.PubHash))
	}
	for _, c := range rec.PubHash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return rec, fmt.Errorf("invalid pk-hash %q (must be lowercase hex)", rec.PubHash)
		}
	}
	if rec.Port <= 0 || rec.Port > 65535 {
		return rec, fmt.Errorf("invalid port %d", rec.Port)
	}
	return rec, nil
}
