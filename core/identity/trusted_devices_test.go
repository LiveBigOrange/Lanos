package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanos/lanos/core/transport"
)

func newTestTrustStore(t *testing.T) *TrustStore {
	t.Helper()
	return NewTrustStoreAt(filepath.Join(t.TempDir(), "trusted_devices.json"))
}

// genStatic derives an X25519 static keypair from a fresh ed25519 key, to
// simulate what a peer would present after a Noise handshake.
func genStatic(t *testing.T) (deviceID string, edPub []byte, static transport.StaticKeys) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen ed25519: %v", err)
	}
	sk, err := transport.DeriveStaticKeys(priv)
	if err != nil {
		t.Fatalf("derive static: %v", err)
	}
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8]), []byte(pub), sk
}

func TestTrustStoreUnknownPeer(t *testing.T) {
	s := newTestTrustStore(t)
	id, _, sk := genStatic(t)
	v, _, err := s.VerifyPeer(id, sk.Public[:])
	if err != nil {
		t.Fatalf("VerifyPeer: %v", err)
	}
	if v != PeerUnknown {
		t.Fatalf("got %v, want PeerUnknown", v)
	}
}

func TestTrustStoreTrustedPeer(t *testing.T) {
	s := newTestTrustStore(t)
	id, edPub, sk := genStatic(t)
	if err := s.Trust(id, sk.Public[:], edPub, "peer", false); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	v, td, err := s.VerifyPeer(id, sk.Public[:])
	if err != nil {
		t.Fatalf("VerifyPeer: %v", err)
	}
	if v != PeerTrusted {
		t.Fatalf("got %v, want PeerTrusted", v)
	}
	if td.Name != "peer" {
		t.Fatalf("name = %q, want peer", td.Name)
	}
}

func TestTrustStoreKeyMismatchDowngrade(t *testing.T) {
	s := newTestTrustStore(t)
	id, edPub, sk := genStatic(t)
	if err := s.Trust(id, sk.Public[:], edPub, "peer", false); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	// Peer rotates identity: new ed25519 key, new X25519 static, but happens to
	// reuse the same deviceID slot (e.g. same device name lookup). In practice
	// the deviceID changes too, but the downgrade path is: same deviceID, new
	// static key.
	_, _, sk2 := genStatic(t)
	v, _, err := s.VerifyPeer(id, sk2.Public[:])
	if err != nil {
		t.Fatalf("VerifyPeer: %v", err)
	}
	if v != PeerKeyMismatch {
		t.Fatalf("got %v, want PeerKeyMismatch (downgrade)", v)
	}
}

func TestTrustStoreReconfirmOverwrites(t *testing.T) {
	s := newTestTrustStore(t)
	id, edPub, sk := genStatic(t)
	if err := s.Trust(id, sk.Public[:], edPub, "peer", false); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	// After downgrade, user re-confirms; overwrite with the new key.
	_, edPub2, sk2 := genStatic(t)
	if err := s.Trust(id, sk2.Public[:], edPub2, "peer", true); err != nil {
		t.Fatalf("re-Trust: %v", err)
	}
	v, td, err := s.VerifyPeer(id, sk2.Public[:])
	if err != nil {
		t.Fatalf("VerifyPeer: %v", err)
	}
	if v != PeerTrusted {
		t.Fatalf("got %v, want PeerTrusted after re-confirm", v)
	}
	if !td.AutoReceive {
		t.Fatal("AutoReceive not updated on re-confirm")
	}
}

func TestTrustStoreRemove(t *testing.T) {
	s := newTestTrustStore(t)
	id, edPub, sk := genStatic(t)
	if err := s.Trust(id, sk.Public[:], edPub, "peer", false); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	if err := s.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	v, _, err := s.VerifyPeer(id, sk.Public[:])
	if err != nil {
		t.Fatalf("VerifyPeer: %v", err)
	}
	if v != PeerUnknown {
		t.Fatalf("got %v, want PeerUnknown after remove", v)
	}
	// Removing a non-existent entry is not an error.
	if err := s.Remove(id); err != nil {
		t.Fatalf("Remove non-existent: %v", err)
	}
}

func TestTrustStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trusted_devices.json")
	s1 := NewTrustStoreAt(path)
	id, edPub, sk := genStatic(t)
	if err := s1.Trust(id, sk.Public[:], edPub, "peer", false); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	// Reopen the same path: the record must survive.
	s2 := NewTrustStoreAt(path)
	v, td, err := s2.VerifyPeer(id, sk.Public[:])
	if err != nil {
		t.Fatalf("VerifyPeer: %v", err)
	}
	if v != PeerTrusted {
		t.Fatalf("got %v, want PeerTrusted after reload", v)
	}
	if td.Name != "peer" {
		t.Fatalf("name = %q after reload", td.Name)
	}
}

func TestTrustStoreCorruptFile(t *testing.T) {
	s := newTestTrustStore(t)
	if err := os.WriteFile(s.Path(), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	if _, _, err := s.VerifyPeer("any", make([]byte, 32)); err != ErrTrustStoreCorrupt {
		t.Fatalf("got err=%v, want ErrTrustStoreCorrupt", err)
	}
}

func TestTrustStoreRejectsBadStaticLen(t *testing.T) {
	s := newTestTrustStore(t)
	if err := s.Trust("id", make([]byte, 31), nil, "peer", false); err == nil {
		t.Fatal("Trust with 31-byte static key should fail")
	}
}
