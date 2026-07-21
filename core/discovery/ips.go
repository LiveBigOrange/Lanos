package discovery

import (
	"net"
	"strings"
)

// detectIPVersion scans non-loopback interfaces and returns:
//
//	"4"  – only IPv4 unicast addresses found
//	"6"  – only IPv6 global unicast addresses found
//	"46" – both IPv4 and IPv6 global unicast found
//
// Link-local and loopback addresses are ignored. Returns "4" as a safe
// fallback when no usable address is found (e.g. offline boot).
func detectIPVersion() string {
	has4, has6 := false, false
	for _, ip := range localUnicastIPs() {
		if ip.IsLoopback() {
			continue
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			has4 = true
		} else if ip.IsGlobalUnicast() {
			has6 = true
		}
	}
	switch {
	case has4 && has6:
		return "46"
	case has6:
		return "6"
	default:
		return "4"
	}
}

// localUnicastIPs returns all non-loopback unicast IPs across all interfaces.
// Used to populate RegisterProxy's host IPs so peers can connect without
// relying on hostname resolution (which often fails in containers).
func localUnicastIPs() []net.IP {
	var out []net.IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			out = append(out, ip)
		}
	}
	return out
}

// localIPv4Strings / localIPv6Strings return deduplicated string forms.
func localIPv4Strings() []string {
	var out []string
	for _, ip := range localUnicastIPs() {
		if ip.To4() != nil && !ip.IsLinkLocalUnicast() {
			out = append(out, ip.String())
		}
	}
	return dedupStrings(out)
}

func localIPv6Strings() []string {
	var out []string
	for _, ip := range localUnicastIPs() {
		if ip.To4() == nil && ip.IsGlobalUnicast() {
			out = append(out, ip.String())
		}
	}
	return dedupStrings(out)
}

// localHostname returns the machine's mDNS-style hostname (without trailing dot).
func localHostname() string {
	host, err := osHostname()
	if err != nil || host == "" {
		host = "lanos-host"
	}
	return strings.TrimSuffix(host, ".")
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
