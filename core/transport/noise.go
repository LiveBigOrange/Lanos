// Package transport implements the Noise XX encrypted transport for Lanos
// device-to-device connections.
//
// Protocol note: docs/PROTOCOL.md §3 labels the handshake "Noise XK", but the
// documented message flow (-> e, <- e, ee, s, es, -> s, se) has no pre-message
// static key, which is the Noise **XX** pattern. XX is required because mDNS
// only broadcasts pk-hash (not the full pubkey), so the initiator cannot know
// the responder's static key ahead of time. XX lets both static keys be
// exchanged encrypted during the handshake; the learned peer static is then
// verified against the mDNS pk-hash and/or trusted_devices.json.
package transport

import (
	"crypto/ed25519"
	"crypto/sha512"
	"errors"
	"fmt"

	"github.com/flynn/noise"
	"golang.org/x/crypto/curve25519"
)

// CipherSuite used by Lanos: X25519 DH + chacha20-poly1305 + SHA256.
// Matches PRD §3.2.1 / docs/PROTOCOL.md §3.1.
var lanosCipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)

// ErrHandshakeIncomplete is returned when an operation is attempted before the
// Noise handshake has finished.
var ErrHandshakeIncomplete = errors.New("transport: handshake not yet complete")

// ErrHandshakeDone is returned when WriteMessage/ReadMessage is called after
// the handshake already completed.
var ErrHandshakeDone = errors.New("transport: handshake already complete")

// StaticKeys is the X25519 static keypair derived from the device's ed25519
// identity key. It is the Noise-level static key used in the XX handshake.
type StaticKeys struct {
	Private [32]byte
	Public  [32]byte
}

// DeriveStaticKeys derives an X25519 static keypair from an ed25519 private key
// using the libsodium-compatible conversion (crypto_sign_ed25519_sk_to_curve25519):
//
//	seed = ed25519_priv[:32]
//	h   = SHA512(seed)
//	h[0] &= 248; h[31] &= 127; h[31] |= 64   (Curve25519 scalar clamping)
//	x25519_priv = h[:32]
//	x25519_pub  = curve25519.ScalarBaseMult(x25519_priv)
//
// This keeps a single long-term identity key (ed25519) while providing the
// X25519 keypair Noise requires for DH. The conversion is deterministic, so
// the same ed25519 key always yields the same X25519 static keypair — letting
// trusted devices recognize each other across restarts.
func DeriveStaticKeys(edPriv ed25519.PrivateKey) (StaticKeys, error) {
	if len(edPriv) != ed25519.PrivateKeySize {
		return StaticKeys{}, fmt.Errorf("transport: invalid ed25519 private key size %d", len(edPriv))
	}
	seed := edPriv.Seed() // first 32 bytes
	h := sha512.Sum512(seed)
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64

	var sk StaticKeys
	copy(sk.Private[:], h[:32])
	curve25519.ScalarBaseMult(&sk.Public, &sk.Private)
	return sk, nil
}

// HandshakeRole identifies whether this side initiates or responds to the
// Noise XX handshake.
type HandshakeRole int

const (
	RoleInitiator HandshakeRole = iota
	RoleResponder
)

// Handshake wraps a flynn/noise HandshakeState for the Lanos XX exchange.
//
// Message flow (Noise XX, 1.5-RTT / 3 messages):
//
//  1. initiator -> responder :  e                 (WriteMessage #1)
//  2. responder -> initiator :  e, ee, s, es      (WriteMessage #2, completes responder side)
//  3. initiator -> responder :  s, se             (WriteMessage #3, completes initiator side)
//
// After message 3 both sides hold matching send/recv CipherStates and the same
// ChannelBinding (handshake hash), from which the SAS code is derived.
type Handshake struct {
	hs     *noise.HandshakeState
	role   HandshakeRole
	step   int // next WriteMessage index expected for this role
	done   bool
	sendCS *noise.CipherState
	recvCS *noise.CipherState
}

// NewHandshake creates a Noise XX handshake. The static X25519 keypair must be
// derived from the local ed25519 identity via DeriveStaticKeys.
func NewHandshake(role HandshakeRole, static StaticKeys) (*Handshake, error) {
	cfg := noise.Config{
		CipherSuite:   lanosCipherSuite,
		Pattern:       noise.HandshakeXX,
		Initiator:     role == RoleInitiator,
		StaticKeypair: noise.DHKey{Private: static.Private[:], Public: static.Public[:]},
	}
	hs, err := noise.NewHandshakeState(cfg)
	if err != nil {
		return nil, fmt.Errorf("transport: new handshake: %w", err)
	}
	return &Handshake{hs: hs, role: role}, nil
}

// WriteMessage produces the next outbound handshake message. It returns the
// message bytes to send to the peer. When the handshake completes on this
// side, the returned send/recv CipherStates are non-nil and stored on the
// Handshake for later use.
//
// payload may be nil for Lanos handshakes (no piggybacked data).
//
// flynn/noise returns (cs1, cs2) regardless of role, where cs1 is the
// initiator->responder direction and cs2 is responder->initiator. We
// normalize so callers always receive (send, recv) for THEIR direction.
func (h *Handshake) WriteMessage(payload []byte) (msg []byte, send, recv *noise.CipherState, err error) {
	if h.done {
		return nil, nil, nil, ErrHandshakeDone
	}
	out, cs1, cs2, err := h.hs.WriteMessage(nil, payload)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("transport: write message %d: %w", h.step, err)
	}
	h.step++
	send, recv = h.normalize(cs1, cs2)
	if send != nil && recv != nil {
		h.done = true
		h.sendCS = send
		h.recvCS = recv
	}
	return out, send, recv, nil
}

// ReadMessage processes an inbound handshake message from the peer. When the
// handshake completes on this side, the returned send/recv CipherStates are
// non-nil and stored on the Handshake.
func (h *Handshake) ReadMessage(message []byte) (payload []byte, send, recv *noise.CipherState, err error) {
	if h.done {
		return nil, nil, nil, ErrHandshakeDone
	}
	out, cs1, cs2, err := h.hs.ReadMessage(nil, message)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("transport: read message %d: %w", h.step, err)
	}
	h.step++
	send, recv = h.normalize(cs1, cs2)
	if send != nil && recv != nil {
		h.done = true
		h.sendCS = send
		h.recvCS = recv
	}
	return out, send, recv, nil
}

// normalize maps flynn/noise's (cs1, cs2) - where cs1 is the initiator->responder
// direction and cs2 is responder->initiator - to (send, recv) for this side.
// cs1/cs2 are nil until the final handshake message; nils pass through.
func (h *Handshake) normalize(cs1, cs2 *noise.CipherState) (send, recv *noise.CipherState) {
	if cs1 == nil || cs2 == nil {
		return cs1, cs2
	}
	if h.role == RoleInitiator {
		return cs1, cs2 // cs1 = send (i->r), cs2 = recv (r->i)
	}
	return cs2, cs1 // responder: cs2 = send (r->i), cs1 = recv (i->r)
}

// Complete reports whether the handshake has finished.
func (h *Handshake) Complete() bool { return h.done }

// ChannelBinding returns the handshake hash (ChannelBinding) once the
// handshake is complete. Both sides derive identical values; it is the input
// to the SAS code. Returns ErrHandshakeIncomplete before completion.
func (h *Handshake) ChannelBinding() ([]byte, error) {
	if !h.done {
		return nil, ErrHandshakeIncomplete
	}
	return h.hs.ChannelBinding(), nil
}

// PeerStatic returns the peer's X25519 static public key learned during the
// XX handshake. Available once the handshake has progressed far enough to
// receive the peer's static (after message 2 for the initiator, after
// message 2 for the responder). Returns nil before that point.
func (h *Handshake) PeerStatic() []byte {
	return h.hs.PeerStatic()
}

// CipherStates returns the send and recv CipherStates after the handshake
// completes. The initiator's send is the responder's recv and vice versa.
func (h *Handshake) CipherStates() (send, recv *noise.CipherState, error error) {
	if !h.done {
		return nil, nil, ErrHandshakeIncomplete
	}
	return h.sendCS, h.recvCS, nil
}
