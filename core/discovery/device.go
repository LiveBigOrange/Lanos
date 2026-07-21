package discovery

import (
	"net"
	"sort"
	"time"
)

// Device represents a peer discovered on the local network.
type Device struct {
	// ID is the device identifier derived from pk-hash (first 16 hex chars).
	// Used as the map key and to deduplicate announcements.
	ID string `json:"id"`

	// Name is the human-readable device name (URL-decoded).
	Name string `json:"name"`

	// Platform is the peer OS (linux/darwin/windows/android/ios).
	Platform string `json:"platform"`

	// Port is the peer's Lanos API+transfer TCP port.
	Port int `json:"port"`

	// PubHash is the peer's public-key hash (16 hex chars).
	PubHash string `json:"pub_hash"`

	// IPVersion indicates the peer's advertised IP capability: "4" | "6" | "46".
	IPVersion string `json:"ip_version"`

	// IPv4 / IPv6 are the resolved addresses for the peer.
	// May be empty until the resolver completes A/AAAA lookups.
	IPv4 []string `json:"ipv4,omitempty"`
	IPv6 []string `json:"ipv6,omitempty"`

	// HostName is the mDNS hostname announced by the peer (e.g. "macbook.local.").
	HostName string `json:"hostname,omitempty"`

	// FirstSeen / LastSeen are timestamps of the first and most recent
	// announcements received from the peer.
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`

	// Status is the liveness state derived from active probing. It is
	// "online" while the peer responds to liveness pings (or has never been
	// probed yet), and "gray" after consecutive probe failures indicate the
	// peer is likely unreachable (e.g. network cable unplugged) even though
	// it has not yet timed out of the mDNS cache. See PRD P1-13.
	Status string `json:"status"`
}

// Liveness status values for Device.Status.
const (
	// StatusOnline means the peer is reachable (responds to liveness pings or
	// was just discovered).
	StatusOnline = "online"
	// StatusGray means liveness probes have failed repeatedly; the peer is
	// likely unreachable but has not yet been evicted from the device list.
	StatusGray = "gray"
)

// IsSelf returns true if this device matches the local identity's pub-hash.
func (d *Device) IsSelf(localPubHash string) bool {
	return d != nil && d.PubHash == localPubHash
}

// SortDevices sorts a slice of devices by (Name, ID) for stable output.
func SortDevices(devs []*Device) {
	sort.Slice(devs, func(i, j int) bool {
		if devs[i].Name != devs[j].Name {
			return devs[i].Name < devs[j].Name
		}
		return devs[i].ID < devs[j].ID
	})
}

// ipsToStrings converts a slice of net.IP to a deduplicated, sorted slice of
// strings (using the canonical form returned by net.IP.String).
func ipsToStrings(ips []net.IP) []string {
	seen := map[string]bool{}
	var out []string
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		s := ip.String()
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
