package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestHandleDiagnostics_RequiresAuth(t *testing.T) {
	s := newTestServer(t, nil)
	rr := doRequest(t, s, "GET", "/api/v1/diagnostics", "", "")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleDiagnostics_ReturnsShape(t *testing.T) {
	s := newTestServer(t, nil)
	rr := doRequest(t, s, "GET", "/api/v1/diagnostics", "test-token-xyz", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		IPVersion  string `json:"ip_version"`
		Interfaces []any  `json:"interfaces"`
		SourceIPs  []any  `json:"source_ips"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rr.Body.String())
	}
	switch resp.IPVersion {
	case "4", "6", "46":
	default:
		t.Errorf("ip_version = %q, want one of 4/6/46", resp.IPVersion)
	}
	if resp.Interfaces == nil {
		t.Errorf("interfaces should be a non-nil array; body=%s", rr.Body.String())
	}
	// Source IPs may legitimately be empty in restricted envs, but the field
	// should always exist in the response.
	if !strings.Contains(rr.Body.String(), "source_ips") {
		t.Errorf("response missing source_ips; body=%s", rr.Body.String())
	}
}
