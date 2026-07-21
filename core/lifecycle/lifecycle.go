// Package lifecycle implements the gcd <-> Flutter startup handshake.
// See PRD §5.1.3 and §5.1.5.
//
//   - NewToken: generates a 32-byte random Bearer token (in-memory only,
//     never persisted).
//   - BindLocal: binds 127.0.0.1:<port> with port 0 meaning "pick random in
//     52100-52999".
//   - EmitHandshake: writes one JSON line to stdout:
//     {"port":52103,"api_token":"<base64>","version":"0.1.0"}
//     Flutter reads this line from the spawned process's stdout pipe.
package lifecycle

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
)

// PortRangeMin / Max define the random port window persisted to config.yaml.
const (
	PortRangeMin = 52100
	PortRangeMax = 52999
)

// NewToken returns a 32-byte cryptographically random token, base64 encoded
// (43 chars, URL-safe). Used as the Bearer token for the local REST API.
func NewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// BindLocal binds 127.0.0.1:<port>. If port is 0, a random port in
// [PortRangeMin, PortRangeMax] is chosen (first available tried in order).
// The returned net.Listener is owned by the caller.
//
// MVP binds IPv4 loopback only. IPv6 dual-stack ([::]) lands in P3 W9
// (see PRD §3.1.8 and core/net package).
func BindLocal(port int) (int, net.Listener, error) {
	if port != 0 && (port < PortRangeMin || port > PortRangeMax) {
		return 0, nil, fmt.Errorf("port %d outside allowed range %d-%d", port, PortRangeMin, PortRangeMax)
	}

	// Try the configured/persisted port first, then scan the range.
	candidates := []int{}
	if port != 0 {
		candidates = append(candidates, port)
	} else {
		for p := PortRangeMin; p <= PortRangeMax; p++ {
			candidates = append(candidates, p)
		}
	}

	var lastErr error
	for _, p := range candidates {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return p, ln, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no available port")
	}
	return 0, nil, fmt.Errorf("bind local: %w", lastErr)
}

// HandshakeMessage is the JSON payload emitted on stdout for Flutter.
type HandshakeMessage struct {
	Port           int    `json:"port"`
	APIToken       string `json:"api_token"`
	Version        string `json:"version"`
	AlreadyRunning bool   `json:"already_running,omitempty"`
}

// EmitHandshake writes the startup handshake JSON line to stdout.
// Exactly one line, no trailing whitespace beyond a single newline,
// so Flutter can bufio-scan it cleanly.
func EmitHandshake(port int, token, version string) error {
	msg := HandshakeMessage{Port: port, APIToken: token, Version: version}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal handshake: %w", err)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}
	return nil
}

// EmitHandshakeAlreadyRunning writes a handshake line indicating another
// gcd instance is already running. Flutter uses this to show a user-friendly
// error instead of "Bad state: No element".
func EmitHandshakeAlreadyRunning(version string) {
	msg := HandshakeMessage{Version: version, AlreadyRunning: true}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	os.Stdout.Write(append(data, '\n'))
}
