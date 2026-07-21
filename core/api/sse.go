package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/lanos/lanos/core/discovery"
)

// EventSource is the subset of *discovery.Discovery needed to stream presence
// events. Declared as an interface so tests can substitute a fake.
type EventSource interface {
	Events() <-chan discovery.Event
}

// sseThrottle is the minimum interval between SSE writes per subscriber, per
// PRD P1-10 ("100ms 节流"). Bursts are coalesced: only the latest event for a
// given (type, device ID) is kept within a throttle window.
const sseThrottle = 100 * time.Millisecond

// eventBroker fans out discovery events to multiple SSE subscribers, applying
// a per-subscriber 100ms throttle. The broker is fed by a single goroutine
// reading from the EventSource; each subscriber gets its own buffered channel.
type eventBroker struct {
	source EventSource

	mu     sync.Mutex
	subs   map[chan discovery.Event]struct{}
	closed chan struct{}
}

func newEventBroker(source EventSource) *eventBroker {
	b := &eventBroker{
		source: source,
		subs:   make(map[chan discovery.Event]struct{}),
		closed: make(chan struct{}),
	}
	if source != nil {
		go b.run()
	}
	return b
}

func (b *eventBroker) run() {
	src := b.source.Events()
	for {
		select {
		case <-b.closed:
			return
		case ev, ok := <-src:
			if !ok {
				return
			}
			b.mu.Lock()
			for ch := range b.subs {
				// Non-blocking: if a subscriber is slow, drop the event for it
				// rather than blocking the whole fan-out. SSE is best-effort.
				select {
				case ch <- ev:
				default:
				}
			}
			b.mu.Unlock()
		}
	}
}

// subscribe registers a new subscriber and returns its event channel and a
// cancel func. The channel is buffered so brief bursts don't drop. The
// caller must call cancel when the SSE stream ends to avoid leaks.
func (b *eventBroker) subscribe() (<-chan discovery.Event, func()) {
	ch := make(chan discovery.Event, 32)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()

	}
	return ch, cancel
}

// handleEvents streams device presence events to the client as Server-Sent
// Events (text/event-stream). See PRD §5.4 GET /api/v1/events.
//
// Event format:
//
//	event: device.online
//	data: {"type":"online","device":{...}}
//
//	event: device.offline
//	data: {"type":"offline","device":{...}}
//
// A 100ms throttle coalesces bursts: when events arrive faster than the
// throttle interval, only the latest event per (type, device ID) is forwarded
// within each window. This keeps the UI responsive without flooding it during
// a large mDNS burst.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Broker == nil {
		http.Error(w, "events unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := s.cfg.Broker.subscribe()
	defer cancel()

	// Send an initial hello so the client knows the stream is alive and can
	// detect connection success before the first device event.
	writeSSE(w, "hello", map[string]any{"ok": true})
	flusher.Flush()

	ctx := r.Context()
	throttle := time.NewTicker(sseThrottle)
	defer throttle.Stop()

	// pending holds coalesced events keyed by "type:deviceID" awaiting the next
	// throttle tick.
	pending := make(map[string]discovery.Event)
	order := make([]string, 0) // preserve insertion order of keys

	flush := func() {
		if len(pending) == 0 {
			return
		}
		for _, key := range order {
			ev := pending[key]
			writeSSE(w, sseEventName(ev.Type), ssePayload{Type: ev.Type, Device: ev.Device})
		}
		pending = make(map[string]discovery.Event)
		order = order[:0]
		flusher.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			key := fmt.Sprintf("%s:%s", ev.Type, ev.Device.ID)
			if _, exists := pending[key]; !exists {
				order = append(order, key)
			}
			pending[key] = ev
		case <-throttle.C:
			flush()
		}
	}
}

type ssePayload struct {
	Type   discovery.EventType `json:"type"`
	Device *discovery.Device   `json:"device"`
}

// sseEventName maps a discovery EventType to the SSE event name. PRD §5.4
// uses the "device.<type>" naming convention.
func sseEventName(t discovery.EventType) string {
	return "device." + string(t)
}

// writeSSE writes a single SSE event to the writer. Each event is one
// "event:" line, one or more "data:" lines, and a blank line terminator.
func writeSSE(w io.Writer, event string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", payload)
}
