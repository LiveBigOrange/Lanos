package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lanos/lanos/core/discovery"
)

// fakeLister is a stand-in for *discovery.Discovery used to test /devices
// without booting real mDNS.
type fakeLister struct {
	self  *discovery.Device
	peers []*discovery.Device
}

func (f *fakeLister) Self() *discovery.Device      { return f.self }
func (f *fakeLister) Devices() []*discovery.Device { return f.peers }

func newTestServer(t *testing.T, lister DeviceLister) *Server {
	t.Helper()
	return NewServer(Config{
		Version:   "0.1.0-test",
		Token:     "test-token-xyz",
		Discovery: lister,
	})
}

func doRequest(t *testing.T, s *Server, method, path, token, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rr := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(rr, req)
	return rr
}

func TestHandleDevices_NoDiscovery(t *testing.T) {
	s := newTestServer(t, nil)
	rr := doRequest(t, s, "GET", "/api/v1/devices", "test-token-xyz", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Self  any   `json:"self"`
		Peers []any `json:"peers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rr.Body.String())
	}
	if resp.Self != nil {
		t.Errorf("self = %v, want nil", resp.Self)
	}
	if resp.Peers == nil || len(resp.Peers) != 0 {
		t.Errorf("peers = %v, want empty non-nil array", resp.Peers)
	}
}

func TestHandleDevices_WithLister(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	lister := &fakeLister{
		self: &discovery.Device{
			ID:        "aaaa1111aaaa1111aaaa1111aaaa1111",
			Name:      "MyMac",
			Platform:  "darwin",
			Port:      52100,
			PubHash:   "aaaa1111aaaa1111aaaa1111aaaa1111",
			IPVersion: "46",
			HostName:  "mymac.local.",
			FirstSeen: now,
			LastSeen:  now,
		},
		peers: []*discovery.Device{
			{
				ID:        "bbbb2222bbbb2222bbbb2222bbbb2222",
				Name:      "WinBox",
				Platform:  "windows",
				Port:      52150,
				PubHash:   "bbbb2222bbbb2222bbbb2222bbbb2222",
				IPVersion: "4",
				IPv4:      []string{"192.168.1.20"},
				HostName:  "winbox.local.",
				FirstSeen: now,
				LastSeen:  now,
			},
		},
	}
	s := newTestServer(t, lister)
	rr := doRequest(t, s, "GET", "/api/v1/devices", "test-token-xyz", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Self  *discovery.Device   `json:"self"`
		Peers []*discovery.Device `json:"peers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rr.Body.String())
	}
	if resp.Self == nil || resp.Self.Name != "MyMac" {
		t.Errorf("self = %+v, want MyMac", resp.Self)
	}
	if len(resp.Peers) != 1 {
		t.Fatalf("peers = %d, want 1", len(resp.Peers))
	}
	if resp.Peers[0].Name != "WinBox" {
		t.Errorf("peer[0] name = %q, want WinBox", resp.Peers[0].Name)
	}
	if resp.Peers[0].PubHash != "bbbb2222bbbb2222bbbb2222bbbb2222" {
		t.Errorf("peer[0] pub_hash = %q", resp.Peers[0].PubHash)
	}
	// Verify the response includes the ip-ver field.
	if !strings.Contains(rr.Body.String(), `"ip_version":"4"`) {
		t.Errorf("response missing ip_version field; body=%s", rr.Body.String())
	}
}

func TestHandleDevices_RequiresAuth(t *testing.T) {
	s := newTestServer(t, nil)
	// No token -> 401
	rr := doRequest(t, s, "GET", "/api/v1/devices", "", "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("no-token status = %d, want 401", rr.Code)
	}
	// Wrong token -> 401
	rr = doRequest(t, s, "GET", "/api/v1/devices", "wrong-token", "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("wrong-token status = %d, want 401", rr.Code)
	}
	// Correct token -> 200
	rr = doRequest(t, s, "GET", "/api/v1/devices", "test-token-xyz", "")
	if rr.Code != http.StatusOK {
		t.Errorf("correct-token status = %d, want 200", rr.Code)
	}
}

func TestHandleDevices_RejectsBadOrigin(t *testing.T) {
	s := newTestServer(t, nil)
	rr := doRequest(t, s, "GET", "/api/v1/devices", "test-token-xyz", "http://evil.example.com")
	if rr.Code != http.StatusForbidden {
		t.Errorf("bad-origin status = %d, want 403", rr.Code)
	}
}

func TestHandleDevices_AllowsLocalhostOrigin(t *testing.T) {
	s := newTestServer(t, nil)
	for _, origin := range []string{
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
		"", // no origin header at all
	} {
		rr := doRequest(t, s, "GET", "/api/v1/devices", "test-token-xyz", origin)
		if rr.Code != http.StatusOK {
			t.Errorf("origin %q: status = %d, want 200", origin, rr.Code)
		}
	}
}

// Compile-time check that *fakeLister satisfies DeviceLister.
var _ DeviceLister = (*fakeLister)(nil)
