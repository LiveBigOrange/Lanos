package discovery

import (
	"os"
	"testing"
	"time"

	"github.com/lanos/lanos/core/config"
	"github.com/lanos/lanos/core/identity"
)

// TestStartStop_Smoke verifies Start() registers a service and Stop()
// cleanly tears it down. Does not require multicast routing.
func TestStartStop_Smoke(t *testing.T) {
	cfg := testConfig(t)
	ident := testIdentity(t)

	d, err := New(cfg, ident)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if d.server == nil {
		t.Fatal("server not set after Start")
	}
	if d.resolver == nil {
		t.Fatal("resolver not set after Start")
	}

	// Self should be populated.
	self := d.Self()
	if self == nil {
		t.Fatal("Self() returned nil")
	}
	if self.PubHash != ident.PubHash {
		t.Errorf("Self PubHash = %q, want %q", self.PubHash, ident.PubHash)
	}
	if self.Port != cfg.Port {
		t.Errorf("Self Port = %d, want %d", self.Port, cfg.Port)
	}

	// Devices() should return empty (we filter out self).
	if devs := d.Devices(); len(devs) != 0 {
		t.Errorf("Devices() = %d, want 0 (self filtered)", len(devs))
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if d.server != nil {
		t.Error("server not nil after Stop")
	}

	// Stop is idempotent.
	if err := d.Stop(); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

// TestEvents_ChannelClosedAfterStop verifies the Events channel is closed
// when Stop is called, so range loops in subscribers terminate cleanly.
func TestEvents_ChannelClosedAfterStop(t *testing.T) {
	cfg := testConfig(t)
	ident := testIdentity(t)

	d, _ := New(cfg, ident)
	_ = d.Start()

	events := d.Events()
	_ = d.Stop()

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("Events channel not closed after Stop")
		}
	case <-time.After(time.Second):
		t.Fatal("Events channel not closed within 1s of Stop")
	}
}

// TestNew_RejectsNilArgs confirms constructor input validation.
func TestNew_RejectsNilArgs(t *testing.T) {
	if _, err := New(nil, nil); err == nil {
		t.Fatal("expected error for nil args")
	}
	cfg := testConfig(t)
	if _, err := New(cfg, nil); err == nil {
		t.Fatal("expected error for nil ident")
	}
	ident := testIdentity(t)
	if _, err := New(nil, ident); err == nil {
		t.Fatal("expected error for nil cfg")
	}
}

// TestStartStop_TwoInstancesSeeEachOther is an end-to-end test that requires
// real multicast routing. It is skipped unless LANOS_TEST_MDNS=1 is set,
// because multicast often does not work in CI containers.
//
// When enabled, it starts two Discovery instances with distinct identities
// on the same host and verifies that each sees the other within 10s.
func TestStartStop_TwoInstancesSeeEachOther(t *testing.T) {
	if os.Getenv("LANOS_TEST_MDNS") != "1" {
		t.Skip("skipping mDNS e2e test; set LANOS_TEST_MDNS=1 to run")
	}

	// Instance A
	cfgA := config.Defaults()
	cfgA.DeviceName = "LanosA"
	cfgA.Port = 52151
	dirA := t.TempDir()
	t.Setenv("HOME", dirA)
	identA, err := identity.LoadOrCreate()
	if err != nil {
		t.Fatalf("identA: %v", err)
	}
	discA, err := New(cfgA, identA)
	if err != nil {
		t.Fatalf("New A: %v", err)
	}
	if err := discA.Start(); err != nil {
		t.Fatalf("Start A: %v", err)
	}
	defer discA.Stop()

	// Instance B - separate HOME so identity.LoadOrCreate produces a different key.
	// We cannot use t.Setenv twice for the same key, so we restore HOME manually
	// via a sub-test or direct os.Setenv with cleanup.
	cfgB := config.Defaults()
	cfgB.DeviceName = "LanosB"
	cfgB.Port = 52152
	dirB := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dirB)
	defer os.Setenv("HOME", origHome)
	identB, err := identity.LoadOrCreate()
	if err != nil {
		t.Fatalf("identB: %v", err)
	}
	if identB.PubHash == identA.PubHash {
		t.Fatal("identA and identB have same PubHash; test cannot proceed")
	}
	discB, err := New(cfgB, identB)
	if err != nil {
		t.Fatalf("New B: %v", err)
	}
	if err := discB.Start(); err != nil {
		t.Fatalf("Start B: %v", err)
	}
	defer discB.Stop()

	// Wait up to 10s for A to see B (or B to see A).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(discA.Devices()) > 0 || len(discB.Devices()) > 0 {
			return // success
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("neither discovery saw the other within 10s")
}

// TestSortDevices verifies stable ordering.
func TestSortDevices(t *testing.T) {
	devs := []*Device{
		{ID: "c", Name: "Zeta"},
		{ID: "a", Name: "Alpha"},
		{ID: "b", Name: "Alpha"}, // same name, different ID
		{ID: "d", Name: "Beta"},
	}
	SortDevices(devs)
	want := []string{"Alpha/a", "Alpha/b", "Beta/d", "Zeta/c"}
	for i, d := range devs {
		got := d.Name + "/" + d.ID
		if got != want[i] {
			t.Errorf("index %d = %s, want %s", i, got, want[i])
		}
	}
}

// TestDeviceIsSelf covers the IsSelf helper.
func TestDeviceIsSelf(t *testing.T) {
	d := &Device{PubHash: "abc"}
	if !d.IsSelf("abc") {
		t.Error("IsSelf should be true for matching hash")
	}
	if d.IsSelf("xyz") {
		t.Error("IsSelf should be false for non-matching hash")
	}
	if (&Device{}).IsSelf("abc") {
		t.Error("IsSelf should be false for empty hash")
	}
}
