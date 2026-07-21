package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/lanos/lanos/core/discovery"
)

func TestPeerAddressPrefersV6InDualStack(t *testing.T) {
	d := &discovery.Device{
		Port: 52100,
		IPv4: []string{"192.168.1.20"},
		IPv6: []string{"fd00::20"},
	}
	// Local source dual-stack: v4 + v6 — happy-eyeballs should prefer v6.
	res, err := peerAddressWith(d, []string{"192.168.1.5", "fd00::5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(res.DialAddr, "[") {
		t.Errorf("expected v6 dial addr (bracketed), got %q", res.DialAddr)
	}
	if res.Version != "6" {
		t.Errorf("Version = %q, want \"6\"", res.Version)
	}
}

func TestPeerAddressV4OnlyPeerV4LocalOK(t *testing.T) {
	d := &discovery.Device{Port: 52100, IPv4: []string{"192.168.1.20"}}
	res, err := peerAddressWith(d, []string{"192.168.1.5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.DialAddr != "192.168.1.20:52100" {
		t.Errorf("DialAddr = %q, want 192.168.1.20:52100", res.DialAddr)
	}
	if res.Version != "4" {
		t.Errorf("Version = %q, want \"4\"", res.Version)
	}
}

func TestPeerAddressV6OnlyPeerV4OnlyLocalErrIncompatible(t *testing.T) {
	d := &discovery.Device{Port: 52100, IPv6: []string{"fd00::20"}}
	_, err := peerAddressWith(d, []string{"192.168.1.5"})
	if !errors.Is(err, ErrIncompatibleIPVersion) {
		t.Fatalf("expected ErrIncompatibleIPVersion, got %v", err)
	}
}

func TestPeerAddressV4OnlyPeerV6OnlyLocalErrIncompatible(t *testing.T) {
	d := &discovery.Device{Port: 52100, IPv4: []string{"192.0.2.20"}}
	_, err := peerAddressWith(d, []string{"fd00::5"})
	if !errors.Is(err, ErrIncompatibleIPVersion) {
		t.Fatalf("expected ErrIncompatibleIPVersion, got %v", err)
	}
}

func TestPeerAddressNoAdvertisedAddr(t *testing.T) {
	d := &discovery.Device{Port: 52100}
	_, err := peerAddressWith(d, []string{"192.168.1.5"})
	if !errors.Is(err, ErrNoPeerAddress) {
		t.Fatalf("expected ErrNoPeerAddress, got %v", err)
	}
}

func TestPeerAddressZeroPortRejected(t *testing.T) {
	d := &discovery.Device{Port: 0, IPv4: []string{"192.0.2.1"}}
	_, err := peerAddressWith(d, []string{"192.168.1.5"})
	if !errors.Is(err, ErrNoPeerAddress) {
		t.Fatalf("expected ErrNoPeerAddress for zero port, got %v", err)
	}
}

func TestPeerAddressLinkLocalV6WithZoneMatchesLinkLocalSrc(t *testing.T) {
	d := &discovery.Device{
		Port: 52100,
		IPv6: []string{"fe80::1%eth0"},
	}
	res, err := peerAddressWith(d, []string{"fe80::5", "2001:db8::5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.DialAddr != "[fe80::1%eth0]:52100" {
		t.Errorf("DialAddr = %q, want [fe80::1%%eth0]:52100", res.DialAddr)
	}
	if res.Version != "6" {
		t.Errorf("Version = %q, want \"6\"", res.Version)
	}
}

func TestPeerAddressLinkLocalV6RequiresLinkLocalSrc(t *testing.T) {
	// Peer advertises fe80::1%eth0; local only has global v6 source → unreachable.
	d := &discovery.Device{
		Port: 52100,
		IPv6: []string{"fe80::1%eth0"},
	}
	_, err := peerAddressWith(d, []string{"2001:db8::5"})
	if !errors.Is(err, ErrIncompatibleIPVersion) {
		t.Fatalf("expected ErrIncompatibleIPVersion for unreachable link-local dst, got %v", err)
	}
}
