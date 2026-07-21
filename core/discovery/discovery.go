// Package discovery implements mDNS / DNS-SD service registration and browsing.
// See PRD §3.1.1.
//
// Service type: _lanos._tcp.local.
// TXT records:
//
//	txt-ver=1  proto=lanos/1.0  platform=<os>  port=<int>
//	pk-hash=<32 hex (16 bytes)>  device-name=<urlencoded>  ip-ver=4|6|46
package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/lanos/lanos/core/config"
	"github.com/lanos/lanos/core/identity"
)

// EventType enumerates the lifecycle events emitted by Discovery.
type EventType string

const (
	EventOnline  EventType = "online"  // new device appeared
	EventOffline EventType = "offline" // device disappeared (timeout)
	EventUpdate  EventType = "update"  // existing device announced again with changed fields
)

// Event is a single device presence change.
type Event struct {
	Type   EventType `json:"type"`
	Device *Device   `json:"device"`
}

const (
	// reapInterval is how often the reaper goroutine sweeps stale devices.
	reapInterval = 10 * time.Second
	// offlineTimeout is the PRD §3.1.2 threshold: if no announcement is
	// received from a peer within this window, it is considered offline.
	offlineTimeout = 30 * time.Second
	// eventBufferSize bounds the Event channel. Subscribers that cannot keep
	// up will see dropped events logged but will not block the resolver.
	eventBufferSize = 64
	// browseEntryBuffer bounds the channel between zeroconf and our consumer.
	browseEntryBuffer = 64
)

// Discovery owns the mDNS server (broadcasting our presence) and browser
// (receiving peer announcements). Concurrency-safe after Start.
type Discovery struct {
	cfg   *config.Config
	ident *identity.Identity
	log   *slog.Logger

	server   *zeroconf.Server
	resolver *zeroconf.Resolver

	mu      sync.RWMutex
	devices map[string]*Device // keyed by Device.ID (= pk-hash)
	events  chan Event

	cancel   context.CancelFunc
	closed   chan struct{}
	stopOnce sync.Once
}

// New constructs a Discovery. Does not start any network activity yet.
func New(cfg *config.Config, ident *identity.Identity) (*Discovery, error) {
	if cfg == nil || ident == nil {
		return nil, fmt.Errorf("cfg and ident must not be nil")
	}
	return &Discovery{
		cfg:     cfg,
		ident:   ident,
		log:     slog.Default().With("component", "discovery"),
		devices: make(map[string]*Device),
		events:  make(chan Event, eventBufferSize),
		closed:  make(chan struct{}),
	}, nil
}

// SetLogger overrides the default slog logger.
func (d *Discovery) SetLogger(l *slog.Logger) {
	if l != nil {
		d.log = l.With("component", "discovery")
	}
}

// Start registers our service on _lanos._tcp.local. and begins browsing.
// Returns an error if registration or resolver setup fails; on success the
// discovery runs until Stop is called.
func (d *Discovery) Start() error {
	ipVer := detectIPVersion()

	text, err := BuildTXT(d.cfg, d.ident, ipVer)
	if err != nil {
		return fmt.Errorf("build TXT: %w", err)
	}

	// Collect local IPs to publish via RegisterProxy. We avoid relying on
	// hostname resolution which is unreliable in containers / CI.
	host := localHostname()
	ips := append(localIPv4Strings(), localIPv6Strings()...)
	if len(ips) == 0 {
		// Fall back to Register (which uses hostname). Better than nothing.
		d.log.Warn("no local unicast IPs found; falling back to hostname registration")
		srv, err := zeroconf.Register(d.cfg.DeviceName, ServiceType, ServiceDomain, d.cfg.Port, text, nil)
		if err != nil {
			return fmt.Errorf("zeroconf register: %w", err)
		}
		d.server = srv
	} else {
		srv, err := zeroconf.RegisterProxy(d.cfg.DeviceName, ServiceType, ServiceDomain, d.cfg.Port, host, ips, text, nil)
		if err != nil {
			return fmt.Errorf("zeroconf register proxy: %w", err)
		}
		d.server = srv
	}

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		d.server.Shutdown()
		d.server = nil
		return fmt.Errorf("new resolver: %w", err)
	}
	d.resolver = resolver

	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel

	entries := make(chan *zeroconf.ServiceEntry, browseEntryBuffer)
	if err := d.resolver.Browse(ctx, ServiceType, ServiceDomain, entries); err != nil {
		cancel()
		d.server.Shutdown()
		d.server = nil
		d.resolver = nil
		return fmt.Errorf("resolver browse: %w", err)
	}

	go d.consumeEntries(entries)
	go d.runReaper(ctx)
	go newProber(d).run(ctx)

	d.log.Info("discovery started",
		"device_name", d.cfg.DeviceName,
		"port", d.cfg.Port,
		"ip_ver", ipVer,
		"pub_hash", d.ident.PubHash,
		"host", host,
		"ips", ips,
	)
	return nil
}

// consumeEntries reads ServiceEntry values from the resolver channel and
// updates the device map. Runs until the entries channel is closed (which
// happens when the resolver context is cancelled), then signals Stop().
func (d *Discovery) consumeEntries(entries <-chan *zeroconf.ServiceEntry) {
	defer close(d.closed)
	for entry := range entries {
		d.handleEntry(entry)
	}
}

// handleEntry applies a single ServiceEntry to the device map.
func (d *Discovery) handleEntry(entry *zeroconf.ServiceEntry) {
	if entry == nil {
		return
	}
	rec, err := ParseTXT(entry.Text)
	if err != nil {
		// Skip announcements from peers with malformed TXT records. We log at
		// debug because non-Lanos services may share the same service type
		// during development.
		d.log.Debug("skip malformed TXT", "instance", entry.Instance, "err", err)
		return
	}
	// Ignore our own announcements (they come back via multicast loopback).
	if rec.PubHash == d.ident.PubHash {
		return
	}

	now := time.Now()
	ipv4 := ipsToStrings(entry.AddrIPv4)
	ipv6 := ipsToStrings(entry.AddrIPv6)

	d.mu.Lock()
	existing, ok := d.devices[rec.PubHash]
	if !ok {
		dev := &Device{
			ID:        rec.PubHash,
			Name:      rec.DeviceName,
			Platform:  rec.Platform,
			Port:      rec.Port,
			PubHash:   rec.PubHash,
			IPVersion: rec.IPVersion,
			IPv4:      ipv4,
			IPv6:      ipv6,
			HostName:  entry.HostName,
			FirstSeen: now,
			LastSeen:  now,
			Status:    StatusOnline,
		}
		d.devices[rec.PubHash] = dev
		d.mu.Unlock()
		d.emit(Event{Type: EventOnline, Device: dev})
		return
	}

	// Update in place; emit "update" only when a meaningful field changed.
	changed := false
	if existing.Name != rec.DeviceName {
		existing.Name = rec.DeviceName
		changed = true
	}
	if existing.Platform != rec.Platform {
		existing.Platform = rec.Platform
		changed = true
	}
	if existing.Port != rec.Port {
		existing.Port = rec.Port
		changed = true
	}
	if existing.IPVersion != rec.IPVersion {
		existing.IPVersion = rec.IPVersion
		changed = true
	}
	if !sliceEqual(existing.IPv4, ipv4) {
		existing.IPv4 = ipv4
		changed = true
	}
	if !sliceEqual(existing.IPv6, ipv6) {
		existing.IPv6 = ipv6
		changed = true
	}
	if existing.HostName != entry.HostName {
		existing.HostName = entry.HostName
		changed = true
	}
	existing.LastSeen = now
	// A fresh mDNS announcement means the peer is reachable: clear any gray
	// status set by the liveness prober (PRD P1-13).
	if existing.Status == StatusGray {
		existing.Status = StatusOnline
		changed = true
	}
	d.mu.Unlock()

	if changed {
		// Snapshot for the event so subscribers see consistent state.
		snap := *existing
		d.emit(Event{Type: EventUpdate, Device: &snap})
	}
}

// runReaper periodically sweeps devices whose LastSeen exceeds offlineTimeout.
func (d *Discovery) runReaper(ctx context.Context) {
	t := time.NewTicker(reapInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.reap()
		}
	}
}

func (d *Discovery) reap() {
	now := time.Now()
	cutoff := now.Add(-offlineTimeout)
	var dropped []*Device
	d.mu.Lock()
	for id, dev := range d.devices {
		if dev.LastSeen.Before(cutoff) {
			dropped = append(dropped, dev)
			delete(d.devices, id)
		}
	}
	d.mu.Unlock()
	for _, dev := range dropped {
		snap := *dev
		d.emit(Event{Type: EventOffline, Device: &snap})
		d.log.Info("device offline", "id", dev.ID, "name", dev.Name)
	}
}

// emit pushes an event onto the channel. Non-blocking: if the channel is
// full, the event is dropped and logged (subscribers must keep up).
func (d *Discovery) emit(ev Event) {
	select {
	case d.events <- ev:
	default:
		d.log.Warn("event channel full; dropping", "type", ev.Type, "id", ev.Device.ID)
	}
}

// Events returns a read-only channel of device presence changes. The channel
// is shared; subscribers should not close it.
func (d *Discovery) Events() <-chan Event {
	return d.events
}

// Self returns a snapshot describing the local device. Returns nil before
// Start has been called.
func (d *Discovery) Self() *Device {
	if d.cfg == nil || d.ident == nil {
		return nil
	}
	return &Device{
		ID:        d.ident.PubHash,
		Name:      d.cfg.DeviceName,
		Platform:  platformString(),
		Port:      d.cfg.Port,
		PubHash:   d.ident.PubHash,
		IPVersion: detectIPVersion(),
		IPv4:      localIPv4Strings(),
		IPv6:      localIPv6Strings(),
		HostName:  localHostname(),
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
		Status:    StatusOnline,
	}
}

// Devices returns a sorted snapshot of currently-online peer devices
// (excluding self).
func (d *Discovery) Devices() []*Device {
	d.mu.RLock()
	out := make([]*Device, 0, len(d.devices))
	for _, dev := range d.devices {
		snap := *dev
		out = append(out, &snap)
	}
	d.mu.RUnlock()
	SortDevices(out)
	return out
}

// DeviceCount returns the number of currently-online peers (excluding self).
func (d *Discovery) DeviceCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.devices)
}

// Stop unregisters our service, cancels browsing, and closes the Events
// channel. Safe to call multiple times; subsequent calls are no-ops.
func (d *Discovery) Stop() error {
	d.stopOnce.Do(func() {
		if d.cancel != nil {
			d.cancel()
			d.cancel = nil
		}
		if d.server != nil {
			d.server.Shutdown()
			d.server = nil
		}
		// Wait for the consumer goroutine to drain the entries channel. The
		// resolver closes the channel after Browse's context is cancelled.
		select {
		case <-d.closed:
		case <-time.After(2 * time.Second):
		}
		close(d.events)
	})
	return nil
}

// sliceEqual is a small helper for []string equality (nil and empty are equal).
func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
