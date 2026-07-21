package lifecycle

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewTokenLength(t *testing.T) {
	tok, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	// base64 raw URL encoding of 32 bytes = 43 chars (no padding).
	if len(tok) != 43 {
		t.Errorf("token length = %d, want 43", len(tok))
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("decoded token bytes = %d, want 32", len(raw))
	}
}

func TestNewTokenUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		tok, err := NewToken()
		if err != nil {
			t.Fatalf("NewToken iter %d: %v", i, err)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("token collision at iter %d", i)
		}
		seen[tok] = struct{}{}
	}
}

func TestBindLocalRandomPort(t *testing.T) {
	port, ln, err := BindLocal(0)
	if err != nil {
		t.Fatalf("BindLocal: %v", err)
	}
	defer ln.Close()
	if port < PortRangeMin || port > PortRangeMax {
		t.Errorf("port = %d, want in [%d,%d]", port, PortRangeMin, PortRangeMax)
	}
}

func TestBindLocalRejectsOutOfRange(t *testing.T) {
	for _, bad := range []int{-1, 1, 80, 52099, 53000, 65536} {
		if _, _, err := BindLocal(bad); err == nil {
			t.Errorf("BindLocal(%d) = nil err, want error", bad)
		}
	}
}

func TestEmitHandshakeJSONShape(t *testing.T) {
	// Capture stdout via a pipe to validate the JSON line shape.
	r, w, err := osPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStdout := swapStdout(w)
	defer restoreStdout(oldStdout)

	if err := EmitHandshake(52103, "test-token-abc", "0.1.0"); err != nil {
		t.Fatalf("EmitHandshake: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	line := strings.TrimRight(string(buf[:n]), "\n")

	var msg HandshakeMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		t.Fatalf("unmarshal %q: %v", line, err)
	}
	if msg.Port != 52103 {
		t.Errorf("Port = %d, want 52103", msg.Port)
	}
	if msg.APIToken != "test-token-abc" {
		t.Errorf("APIToken = %q, want test-token-abc", msg.APIToken)
	}
	if msg.Version != "0.1.0" {
		t.Errorf("Version = %q, want 0.1.0", msg.Version)
	}
}
