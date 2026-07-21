package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestProberMarksGrayAfterFailures verifies that a peer that stops responding
// is marked gray after maxProbeFailures consecutive probe failures.
func TestProberMarksGrayAfterFailures(t *testing.T) {
	// A server we can shut down to simulate a dead peer.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := &Discovery{
		devices: map[string]*Device{
			"dev1": {
				ID:       "dev1",
				Name:     "Peer",
				Port:     0, // overwritten below
				IPv4:     []string{srv.Listener.Addr().String()},
				Status:   StatusOnline,
				LastSeen: time.Now(),
			},
		},
		events: make(chan Event, 16),
		log:    testLogger(),
	}
	// Extract host:port from the listener address.
	addr := srv.Listener.Addr().String()
	host, port := splitHostPort(t, addr)
	d.devices["dev1"].IPv4 = []string{host}
	d.devices["dev1"].Port = port

	p := newProber(d)
	// First probe while server is up: should succeed, status stays online.
	p.probeOne(context.Background(), d.snapshot("dev1"))
	if dev := d.snapshot("dev1"); dev.Status != StatusOnline {
		t.Fatalf("after successful probe: status=%q, want online", dev.Status)
	}

	// Shut down the server to simulate the peer going dark.
	srv.Close()

	// Probe repeatedly; after maxProbeFailures it should go gray. Each probe
	// fails because the server is down.
	for i := 0; i < maxProbeFailures; i++ {
		p.probeOne(context.Background(), d.snapshot("dev1"))
	}
	if dev := d.snapshot("dev1"); dev.Status != StatusGray {
		t.Fatalf("after %d failures: status=%q, want gray", maxProbeFailures, dev.Status)
	}

	// Failure counter should reflect the accumulated failures.
	p.mu.Lock()
	got := p.failures["dev1"]
	p.mu.Unlock()
	if got < maxProbeFailures {
		t.Fatalf("failure count = %d, want >= %d", got, maxProbeFailures)
	}
}

// TestProberRecoversFromGray verifies that a successful probe resets a gray
// device back to online.
func TestProberRecoversFromGray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := &Discovery{
		devices: map[string]*Device{"dev1": {
			ID:       "dev1",
			Name:     "Peer",
			Status:   StatusGray,
			LastSeen: time.Now(),
		}},
		events: make(chan Event, 16),
		log:    testLogger(),
	}
	host, port := splitHostPort(t, srv.Listener.Addr().String())
	d.devices["dev1"].IPv4 = []string{host}
	d.devices["dev1"].Port = port
	// Seed a failure count so onSuccess sees a change.
	p := newProber(d)
	p.failures["dev1"] = maxProbeFailures

	p.probeOne(context.Background(), d.snapshot("dev1"))
	if dev := d.snapshot("dev1"); dev.Status != StatusOnline {
		t.Fatalf("after recovery probe: status=%q, want online", dev.Status)
	}
	p.mu.Lock()
	got := p.failures["dev1"]
	p.mu.Unlock()
	if got != 0 {
		t.Fatalf("failure count after recovery = %d, want 0", got)
	}
}

// TestProberIgnoresEmptyPeerList verifies probeAll is a no-op with no peers.
func TestProberIgnoresEmptyPeerList(t *testing.T) {
	d := &Discovery{
		devices: map[string]*Device{},
		events:  make(chan Event, 16),
		log:     testLogger(),
	}
	p := newProber(d)
	p.probeAll(context.Background()) // must not panic or block
}

// TestPeerAddrBracketing verifies IPv6 addresses are bracketed.
func TestPeerAddrBracketing(t *testing.T) {
	if got, want := peerAddr("192.168.1.5", 52100), "192.168.1.5:52100"; got != want {
		t.Fatalf("ipv4 addr = %q, want %q", got, want)
	}
	if got, want := peerAddr("fe80::1", 52100), "[fe80::1]:52100"; got != want {
		t.Fatalf("ipv6 addr = %q, want %q", got, want)
	}
}

// splitHostPort splits a host:port string for the test server address.
func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, port, err := netSplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	return host, port
}
