package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Liveness probing parameters (PRD P1-13: "mDNS TTL + 5s ICMP ping + 3 次失败
// 标灰"). We use an HTTP GET to the peer's /api/v1/ping instead of raw ICMP,
// which avoids needing raw-socket privileges on every platform while still
// confirming the peer's gcd process is up and reachable.
const (
	// probeInterval is how often each peer is probed.
	probeInterval = 5 * time.Second
	// probeTimeout is the per-probe deadline. Short so a dead peer is detected
	// within one probe cycle.
	probeTimeout = 2 * time.Second
	// maxProbeFailures is the consecutive failure count before a device is
	// marked gray. 3 failures × 5s = 15s, matching the PRD DoD ("拔网线 15s 后
	// UI 正确标记").
	maxProbeFailures = 3
)

// prober actively probes discovered peers to detect liveness faster than the
// 30s mDNS timeout. It is owned by Discovery and started in Start().
type prober struct {
	disc *Discovery
	log  *slog.Logger
	http *http.Client

	mu       sync.Mutex
	failures map[string]int // device ID -> consecutive failure count
}

func newProber(d *Discovery) *prober {
	return &prober{
		disc:     d,
		log:      d.log.With("subcomponent", "prober"),
		http:     &http.Client{Timeout: probeTimeout},
		failures: map[string]int{},
	}
}

// run probes all known peers every probeInterval until ctx is canceled.
func (p *prober) run(ctx context.Context) {
	t := time.NewTicker(probeInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.probeAll(ctx)
		}
	}
}

// probeAll snapshots the current device list and probes each peer in parallel.
func (p *prober) probeAll(ctx context.Context) {
	peers := p.disc.Devices()
	if len(peers) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, dev := range peers {
		dev := dev
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.probeOne(ctx, dev)
		}()
	}
	wg.Wait()
}

// probeOne pings a single peer and updates its Status on failure/recovery.
func (p *prober) probeOne(ctx context.Context, dev *Device) {
	ok := p.ping(ctx, dev)
	if ok {
		p.onSuccess(dev.ID)
		return
	}
	p.onFailure(dev.ID, dev)
}

// ping returns true if the peer's /api/v1/ping responds with 2xx within
// probeTimeout. It tries IPv4 then IPv6 addresses.
func (p *prober) ping(ctx context.Context, dev *Device) bool {
	urls := p.peerURLs(dev)
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		resp, err := p.http.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true
		}
	}
	return false
}

// peerURLs builds the list of /api/v1/ping URLs to try for a device, IPv4
// first then IPv6 (with bracketing).
func (p *prober) peerURLs(dev *Device) []string {
	var urls []string
	for _, ip := range dev.IPv4 {
		urls = append(urls, fmt.Sprintf("http://%s/ping", peerAddr(ip, dev.Port)))
	}
	for _, ip := range dev.IPv6 {
		urls = append(urls, fmt.Sprintf("http://%s/ping", peerAddr(ip, dev.Port)))
	}
	return urls
}

// onSuccess resets the failure counter and, if the device was gray, restores it
// to online and emits an update event.
func (p *prober) onSuccess(id string) {
	p.mu.Lock()
	had := p.failures[id]
	delete(p.failures, id)
	p.mu.Unlock()
	if had == 0 {
		return
	}
	// Only emit if the device was actually gray.
	if dev := p.disc.snapshot(id); dev != nil && dev.Status == StatusGray {
		p.disc.setStatus(id, StatusOnline)
	}
}

// onFailure increments the failure counter; once it reaches maxProbeFailures
// the device is marked gray and an update event is emitted.
func (p *prober) onFailure(id string, dev *Device) {
	p.mu.Lock()
	p.failures[id]++
	count := p.failures[id]
	p.mu.Unlock()
	if count < maxProbeFailures {
		return
	}
	p.disc.setStatus(id, StatusGray)
}

// snapshot returns a copy of the device with the given ID, or nil if absent.
func (d *Discovery) snapshot(id string) *Device {
	d.mu.RLock()
	defer d.mu.RUnlock()
	dev, ok := d.devices[id]
	if !ok {
		return nil
	}
	snap := *dev
	return &snap
}

// setStatus updates a device's Status and emits an update event if it changed.
func (d *Discovery) setStatus(id, status string) {
	d.mu.Lock()
	dev, ok := d.devices[id]
	if !ok {
		d.mu.Unlock()
		return
	}
	if dev.Status == status {
		d.mu.Unlock()
		return
	}
	dev.Status = status
	snap := *dev
	d.mu.Unlock()
	d.emit(Event{Type: EventUpdate, Device: &snap})
	d.log.Info("device liveness changed", "id", id, "status", status)
}

// peerAddr formats an ip:port pair, bracketing IPv6 addresses.
func peerAddr(ip string, port int) string {
	if isIPv6(ip) {
		return fmt.Sprintf("[%s]:%d", ip, port)
	}
	return fmt.Sprintf("%s:%d", ip, port)
}

// isIPv6 reports whether s looks like an IPv6 address (contains a colon).
func isIPv6(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return true
		}
	}
	return false
}
