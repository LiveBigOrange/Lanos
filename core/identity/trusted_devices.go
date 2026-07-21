package identity

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TrustedDevice is one entry in trusted_devices.json. See docs/PROTOCOL.md §3.4.
//
// The Noise XX handshake exchanges X25519 static keys (the DH key for the
// protocol), not ed25519 keys directly. Lanos derives each device's X25519
// static keypair from its ed25519 identity seed (see transport.DeriveStaticKeys),
// so the X25519 static public key is a stable per-device fingerprint that is
// both available from the handshake and comparable on reconnect. It is stored
// as StaticKey (hex) and used for trust verification. An optional ed25519
// pubkey may be stored for future signing use when exchanged as an
// authenticated post-handshake payload.
type TrustedDevice struct {
	PubKey      string `json:"pubkey,omitempty"` // ed25519 public key hex, when known
	StaticKey   string `json:"static_key"`       // X25519 Noise static pubkey hex (primary trust key)
	Name        string `json:"name"`             // device display name
	TrustedAt   int64  `json:"trusted_at"`       // unix seconds
	AutoReceive bool   `json:"auto_receive"`     // accept incoming transfers without prompt
}

// PeerVerification is the result of checking a connecting peer's identity
// against the trust store.
type PeerVerification int

const (
	// PeerUnknown means no trusted_devices.json entry exists for this device ID.
	// The peer is a stranger: run the SAS confirmation flow.
	PeerUnknown PeerVerification = iota
	// PeerTrusted means the stored ed25519 pubkey matches the peer's Noise
	// static key (derived via X25519). Skip SAS, proceed to transfer.
	PeerTrusted
	// PeerKeyMismatch means a record exists for this device ID but the pubkey
	// differs (peer reinstalled OS / key rotated). Downgrade to stranger:
	// re-run SAS, then overwrite the stored key on re-confirmation.
	PeerKeyMismatch
)

// trustedFileName is the on-disk trust store filename, in the same data
// directory as identity.key.
const trustedFileName = "trusted_devices.json"

// ErrTrustStoreCorrupt is returned when trusted_devices.json exists but
// cannot be parsed.
var ErrTrustStoreCorrupt = errors.New("identity: trusted_devices.json is corrupt")

// TrustStore reads and writes trusted_devices.json.
type TrustStore struct {
	path string
}

// NewTrustStore creates a TrustStore rooted at the platform data directory.
func NewTrustStore() (*TrustStore, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &TrustStore{path: filepath.Join(dir, trustedFileName)}, nil
}

// NewTrustStoreAt creates a TrustStore at an explicit path (for tests).
func NewTrustStoreAt(path string) *TrustStore {
	return &TrustStore{path: path}
}

// Path returns the on-disk file path.
func (s *TrustStore) Path() string { return s.path }

// load reads and parses the store. A missing file yields an empty map and no
// error.
func (s *TrustStore) load() (map[string]TrustedDevice, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]TrustedDevice{}, nil
		}
		return nil, fmt.Errorf("read trust store: %w", err)
	}
	var m map[string]TrustedDevice
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, ErrTrustStoreCorrupt
	}
	if m == nil {
		m = map[string]TrustedDevice{}
	}
	return m, nil
}

// save writes the store atomically (temp file + rename) with 0600 perms.
func (s *TrustStore) save(m map[string]TrustedDevice) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trust store: %w", err)
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".trusted_devices.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp trust store: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if rename succeeded
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp trust store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp trust store: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("chmod temp trust store: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("rename trust store: %w", err)
	}
	return nil
}

// Lookup returns the trusted device record for deviceID (16 hex chars = first
// 8 bytes of SHA256(pubkey)) and whether it existed.
func (s *TrustStore) Lookup(deviceID string) (TrustedDevice, bool, error) {
	m, err := s.load()
	if err != nil {
		return TrustedDevice{}, false, err
	}
	td, ok := m[deviceID]
	return td, ok, nil
}

// Trust adds or overwrites a trusted device record. deviceID is the 16-hex-char
// device identifier. staticKey is the peer's X25519 Noise static public key
// (32 bytes) learned from the completed handshake. An optional ed25519 pubkey
// may be stored for future signing use (pass nil if unknown).
func (s *TrustStore) Trust(deviceID string, staticKey []byte, edPubKey []byte, name string, autoReceive bool) error {
	if len(staticKey) != 32 {
		return fmt.Errorf("identity: static key must be 32 bytes, got %d", len(staticKey))
	}
	m, err := s.load()
	if err != nil {
		return err
	}
	td := TrustedDevice{
		StaticKey:   hex.EncodeToString(staticKey),
		Name:        name,
		TrustedAt:   time.Now().Unix(),
		AutoReceive: autoReceive,
	}
	if len(edPubKey) > 0 {
		td.PubKey = hex.EncodeToString(edPubKey)
	}
	m[deviceID] = td
	return s.save(m)
}

// Remove deletes a trusted device record. Removing a non-existent entry is not
// an error.
func (s *TrustStore) Remove(deviceID string) error {
	m, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := m[deviceID]; !ok {
		return nil
	}
	delete(m, deviceID)
	return s.save(m)
}

// VerifyPeer classifies a peer given its device ID and the X25519 static key
// learned during the Noise XX handshake. The stored static key is compared
// directly. See docs/PROTOCOL.md §3.4.
//
//   - PeerUnknown:     no record -> run SAS
//   - PeerTrusted:     static key matches -> skip SAS
//   - PeerKeyMismatch: record exists but key differs -> downgrade, re-run SAS
func (s *TrustStore) VerifyPeer(deviceID string, peerX25519Static []byte) (PeerVerification, TrustedDevice, error) {
	td, ok, err := s.Lookup(deviceID)
	if err != nil {
		return PeerUnknown, TrustedDevice{}, err
	}
	if !ok {
		return PeerUnknown, TrustedDevice{}, nil
	}
	stored, err := hex.DecodeString(td.StaticKey)
	if err != nil || len(stored) != 32 || len(peerX25519Static) != 32 {
		// Stored key malformed or peer key wrong size: treat as mismatch so the
		// user re-confirms and the record gets overwritten.
		return PeerKeyMismatch, td, nil
	}
	if equalBytes(stored, peerX25519Static) {
		return PeerTrusted, td, nil
	}
	return PeerKeyMismatch, td, nil
}

// equalBytes is a constant-time-ish byte comparison. We do not import
// crypto/subtle here because the static keys are public (not secrets), so a
// plain comparison is acceptable; this helper just keeps the call site tidy.
func equalBytes(a, b []byte) bool {
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

// List returns all trusted devices keyed by device ID.
func (s *TrustStore) List() (map[string]TrustedDevice, error) {
	return s.load()
}
