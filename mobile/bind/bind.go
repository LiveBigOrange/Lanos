// Package bind is the gomobile-bind entry surface for the Lanos mobile
// runtimes (Android AAR, iOS XCFramework). It exposes a thin facade over
// usecase.Sender / Receiver, the lanos:// URI parser, and the RFC 6724
// address selection helper.
//
// Signatures conform to gomobile's bind type subset: exported types only,
// basic scalar params, single string returns, error for exceptions. Slice
// parameters are peeled from CSV-encoded string inputs (the simplest type
// bind-friendly representation).
//
// Integration: build with `gomobile bind -target=android/arm64
// github.com/lanos/lanos/mobile/bind` to produce a runnable AAR. The Go
// code itself builds with plain `go build` so desktop toolchains can run
// the tests without NDK setup.
package bind

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lanos/lanos/core/discovery"
	lanosnet "github.com/lanos/lanos/core/net"
	"github.com/lanos/lanos/core/transport"
	"github.com/lanos/lanos/core/usecase"
)

// Bridge is the dispatch surface injected into the mobile runtime. The
// mobile caller constructs a Bridge via NewBridge, supplying pre-minted
// usecase implementations plus the device's static keys (hex-encoded to
// survive the bind type funnel — byte arrays would be unwieldy through
// Java/Kotlin).
type Bridge struct {
	sender   usecase.Sender
	receiver usecase.Receiver
	static   transport.StaticKeys
}

// NewBridge constructs a Bridge bound to the given usecase implementations.
// Either sender or receiver may be nil if the mobile build intentionally
// exposes only one direction (post only / receive only).
//
// pubkeyHex / privkeyHex are the 32-byte X25519 static keys (from
// transport.DeriveStaticKeys) as 64-char lowercase hex strings.
func NewBridge(sender usecase.Sender, receiver usecase.Receiver, pubkeyHex, privkeyHex string) (*Bridge, error) {
	sk, err := decodeStaticKeys(pubkeyHex, privkeyHex)
	if err != nil {
		return nil, err
	}
	return &Bridge{sender: sender, receiver: receiver, static: sk}, nil
}

// SendFile dispatches an outbound transfer. Blocks until the send attempt
// completes or the 60s deadline is reached. Returns the usecase error on
// failure (the underlying usecase surfaces PEER_NOT_RESPONDING etc. as
// string-wrapped errors).
func (b *Bridge) SendFile(peerID, peerAddr, peerName, filePath, transferID string) error {
	if b.sender == nil {
		return errors.New("bind: sender not configured")
	}
	timeout := sendTimeout(filePath)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return b.sender.Send(ctx, usecase.SendConfig{
		PeerID:     peerID,
		PeerAddr:   peerAddr,
		PeerName:   peerName,
		FilePath:   filePath,
		StaticKeys: b.static,
		TransferID: transferID,
	})
}

// ReceiveFile dispatches an inbound transfer (dials peer expecting them to
// push). saveDir is the directory the file should land in; the underlying
// usecase derives the file name from the peer's lanos:// header.
func (b *Bridge) ReceiveFile(peerAddr, peerID, peerName, saveDir string) error {
	if b.receiver == nil {
		return errors.New("bind: receiver not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return b.receiver.Receive(ctx, usecase.ReceiveConfig{
		PeerAddr:   peerAddr,
		PeerID:     peerID,
		PeerName:   peerName,
		SaveDir:    saveDir,
		StaticKeys: b.static,
	})
}

// ConnectURIDTO is the export-friendly mirror of lanosnet.ConnectURI. Each
// field maps 1:1 to the parsed URI field (see docs/PROTOCOL.md §2).
type ConnectURIDTO struct {
	IP         string
	IP6        string
	Port       int
	PKHash     string
	DeviceName string
}

// ParseConnectURI parses a lanos://connect URI. Returns a ConnectURIDTO ready
// for UI rendering (DeviceName) or address selection (IP/IP6/Port).
func (b *Bridge) ParseConnectURI(uri string) (*ConnectURIDTO, error) {
	c, err := lanosnet.ParseConnectURI(uri)
	if err != nil {
		return nil, err
	}
	return &ConnectURIDTO{
		IP:         c.IP,
		IP6:        c.IP6,
		Port:       c.Port,
		PKHash:     c.PKHash,
		DeviceName: c.DeviceName,
	}, nil
}

// SelectBestAddress wraps RFC 6724 SelectAddresses. dstsCSV and srcsCSV are
// comma-separated IP strings (link-local v6 may carry zone ids). port is the
// peer's listening port. The returned string is the sorted, newline-joined
// list of dialable addresses (best first), or "" if no compatible address
// exists (INCOMPATIBLE_IP_VERSION at the network layer).
func SelectBestAddress(dstsCSV, srcsCSV string, port int) string {
	pairs := lanosnet.SelectAddresses(splitCSV(dstsCSV), splitCSV(srcsCSV), port)
	var sb strings.Builder
	for i, p := range pairs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(p.Destination)
	}
	return sb.String()
}

// LocalSourceIPsCSV returns the host's candidate source IPs joined by ",".
// Useful for diagnostics rendering and for feeding SelectBestAddress from
// the peer side of a QR handoff.
func LocalSourceIPsCSV() string {
	return strings.Join(discovery.LocalSourceIPs(), ",")
}

// LocalIPVersion returns "4", "6", or "46" — the local host's stack
// capability flag. Mobile UIs surface this to the user to indicate whether
// they should expect v6-only peers to be reachable.
func LocalIPVersion() string {
	return discovery.LocalIPVersion()
}

// splitCSV normalizes the comma-separated form: trims surrounding whitespace
// on each entry and drops empty fields.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// decodeStaticKeys parses two 32-byte hex strings into a StaticKeys struct.
func decodeStaticKeys(pubHex, privHex string) (transport.StaticKeys, error) {
	var sk transport.StaticKeys
	pub, err := hex.DecodeString(pubHex)
	if err != nil {
		return sk, fmt.Errorf("bind: bad public key hex: %w", err)
	}
	if len(pub) != 32 {
		return sk, fmt.Errorf("bind: public key must be 32 bytes, got %d", len(pub))
	}
	priv, err := hex.DecodeString(privHex)
	if err != nil {
		return sk, fmt.Errorf("bind: bad private key hex: %w", err)
	}
	if len(priv) != 32 {
		return sk, fmt.Errorf("bind: private key must be 32 bytes, got %d", len(priv))
	}
	copy(sk.Public[:], pub)
	copy(sk.Private[:], priv)
	return sk, nil
}

func sendTimeout(filePath string) time.Duration {
	info, err := os.Stat(filePath)
	if err != nil {
		return 60 * time.Second
	}
	gb := float64(info.Size()) / float64(1<<30)
	minutes := 1.0 + gb*2.0
	if minutes < 1.0 {
		minutes = 1.0
	}
	if minutes > 60.0 {
		minutes = 60.0
	}
	return time.Duration(minutes * float64(time.Minute))
}
