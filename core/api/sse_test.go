package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lanos/lanos/core/discovery"
)

// fakeEventSource is a controllable EventSource for SSE tests.
type fakeEventSource struct {
	ch chan discovery.Event
}

func newFakeEventSource() *fakeEventSource {
	return &fakeEventSource{ch: make(chan discovery.Event, 16)}
}

func (f *fakeEventSource) Events() <-chan discovery.Event { return f.ch }
func (f *fakeEventSource) emit(ev discovery.Event)        { f.ch <- ev }

// runSSEHandler runs handleEvents in a goroutine against a recorder with a
// cancellable context. It returns the body buffer and a stop func that cancels
// the context and waits for the handler to exit. Callers MUST call stop()
// before reading the body to avoid a data race.
func runSSEHandler(t *testing.T, srv *Server) (*bytes.Buffer, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		srv.handleEvents(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx))
		close(done)
	}()
	// Give the handler a moment to subscribe and write hello.
	time.Sleep(30 * time.Millisecond)
	return rec.Body, func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("handler did not exit after cancel")
		}
	}
}

// TestSSEHello verifies the stream opens with a hello event.
func TestSSEHello(t *testing.T) {
	src := newFakeEventSource()
	srv := NewServer(Config{Token: "tok", EventSource: src})
	body, stop := runSSEHandler(t, srv)
	stop()
	if !strings.Contains(body.String(), "event: hello") {
		t.Fatalf("body missing hello event:\n%s", body.String())
	}
}

// TestSSEStreamsDeviceEvent verifies a device.online event is forwarded.
func TestSSEStreamsDeviceEvent(t *testing.T) {
	src := newFakeEventSource()
	srv := NewServer(Config{Token: "tok", EventSource: src})
	body, stop := runSSEHandler(t, srv)

	src.emit(discovery.Event{
		Type:   discovery.EventOnline,
		Device: &discovery.Device{ID: "abc12345", Name: "Test Device"},
	})
	// Wait past the 100ms throttle tick for the event to flush.
	time.Sleep(200 * time.Millisecond)
	stop()

	out := body.String()
	if !strings.Contains(out, "event: device.online") {
		t.Fatalf("body missing device.online:\n%s", out)
	}
	if !strings.Contains(out, "abc12345") {
		t.Fatalf("body missing device ID:\n%s", out)
	}
}

// TestSSEThrottleCoalesces verifies that 5 rapid events for the same device
// coalesce into a single emission within one throttle window.
func TestSSEThrottleCoalesces(t *testing.T) {
	src := newFakeEventSource()
	srv := NewServer(Config{Token: "tok", EventSource: src})
	body, stop := runSSEHandler(t, srv)

	for i := 0; i < 5; i++ {
		src.emit(discovery.Event{
			Type:   discovery.EventOnline,
			Device: &discovery.Device{ID: "dupdev1", Name: "Dup"},
		})
	}
	time.Sleep(250 * time.Millisecond)
	stop()

	out := body.String()
	count := strings.Count(out, "event: device.online")
	if count != 1 {
		t.Fatalf("coalesced online count = %d, want 1 (throttle should merge duplicates):\n%s", count, out)
	}
}

// TestSSEUnavailableWithoutBroker verifies /events returns 503 when no event
// source is configured.
func TestSSEUnavailableWithoutBroker(t *testing.T) {
	srv := NewServer(Config{Token: "tok"})
	rec := httptest.NewRecorder()
	srv.handleEvents(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
