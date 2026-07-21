package bind

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lanos/lanos/core/usecase"
)

// dummy pub/priv for tests — 32 bytes each, all 0x01 / 0x02.
const (
	testPubHex  = "0101010101010101010101010101010101010101010101010101010101010101" // 64 hex digits = 32 bytes
	testPrivHex = "0202020202020202020202020202020202020202020202020202020202020202" // 64 hex digits = 32 bytes
)

func TestNewBridge_DecodeStaticKeys(t *testing.T) {
	b, err := NewBridge(&fakeSender{}, &fakeReceiver{}, testPubHex, testPrivHex)
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	if b.static.Public[0] != 0x01 || b.static.Private[0] != 0x02 {
		t.Errorf("static keys not decoded: pub=%x priv=%x", b.static.Public[:1], b.static.Private[:1])
	}
}

func TestNewBridge_RejectsBadKeyHex(t *testing.T) {
	for _, bad := range []string{"", "zz", "0102"} {
		if _, err := NewBridge(nil, nil, bad, testPrivHex); err == nil {
			t.Errorf("expected error for pub=%q", bad)
		}
		if _, err := NewBridge(nil, nil, testPubHex, bad); err == nil {
			t.Errorf("expected error for priv=%q", bad)
		}
	}
}

func TestBridge_SendFileRejectsNilSender(t *testing.T) {
	b, _ := NewBridge(nil, nil, testPubHex, testPrivHex)
	if err := b.SendFile("p", "1.1.1.1:1", "n", "f", ""); err == nil {
		t.Errorf("expected error when sender not configured")
	}
}

func TestBridge_SendFileForwardsToSender(t *testing.T) {
	s := &fakeSender{}
	b, _ := NewBridge(s, nil, testPubHex, testPrivHex)
	if err := b.SendFile("p1", "1.1.1.1:1", "alice", "/tmp/x.bin", "tid"); err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	if !s.called {
		t.Fatal("sender not called")
	}
	if s.cfg.PeerID != "p1" || s.cfg.PeerName != "alice" || s.cfg.FilePath != "/tmp/x.bin" || s.cfg.TransferID != "tid" {
		t.Errorf("cfg not propagated: %+v", s.cfg)
	}
	if s.cfg.PeerAddr != "1.1.1.1:1" {
		t.Errorf("PeerAddr = %q", s.cfg.PeerAddr)
	}
	if s.cfg.StaticKeys.Public[0] != 0x01 {
		t.Errorf("StaticKeys not injected from Bridge: %+v", s.cfg.StaticKeys)
	}
}

func TestBridge_SendFileSurfacesSenderError(t *testing.T) {
	want := errors.New("boom")
	s := &fakeSender{err: want}
	b, _ := NewBridge(s, nil, testPubHex, testPrivHex)
	if err := b.SendFile("p", "1.1.1.1:1", "n", "f", ""); !errors.Is(err, want) {
		t.Errorf("expected wrapped boom, got %v", err)
	}
}

func TestBridge_ReceiveFileRejectsNilReceiver(t *testing.T) {
	b, _ := NewBridge(nil, nil, testPubHex, testPrivHex)
	if err := b.ReceiveFile("1.1.1.1:1", "p", "n", "/tmp"); err == nil {
		t.Errorf("expected error when receiver not configured")
	}
}

func TestBridge_ReceiveFileForwardsToReceiver(t *testing.T) {
	r := &fakeReceiver{}
	b, _ := NewBridge(nil, r, testPubHex, testPrivHex)
	if err := b.ReceiveFile("1.1.1.1:1", "p1", "bob", "/tmp/in"); err != nil {
		t.Fatalf("ReceiveFile: %v", err)
	}
	if !r.called {
		t.Fatal("receiver not called")
	}
}

func TestBridge_ParseConnectURI(t *testing.T) {
	b, _ := NewBridge(nil, nil, testPubHex, testPrivHex)
	const raw = "lanos://connect?ip=192.0.2.1&ip6=fd00::1&port=52100&pk-hash=0123456789abcdef0123456789abcdef&device-name=alice"
	dto, err := b.ParseConnectURI(raw)
	if err != nil {
		t.Fatalf("ParseConnectURI: %v", err)
	}
	if dto.IP != "192.0.2.1" || dto.IP6 != "fd00::1" || dto.Port != 52100 || dto.DeviceName != "alice" {
		t.Errorf("DTO mismatch: %+v", dto)
	}
}

func TestBridge_ParseConnectURIInvalid(t *testing.T) {
	b, _ := NewBridge(nil, nil, testPubHex, testPrivHex)
	if _, err := b.ParseConnectURI("not a lanos URI"); err == nil {
		t.Errorf("expected error for invalid URI")
	}
}

func TestSelectBestAddressPrefersV6(t *testing.T) {
	got := SelectBestAddress("192.0.2.10,2001:db8::10", "192.0.2.5,2001:db8::5", 52100)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 dialable results, got %q", got)
	}
	if !strings.HasPrefix(lines[0], "[") {
		t.Errorf("expected v6 dial addr first, got %q", lines[0])
	}
}

func TestSelectBestAddressEmptyOnIncompatible(t *testing.T) {
	got := SelectBestAddress("192.0.2.10", "fd00::5", 52100)
	if got != "" {
		t.Errorf("expected empty for v4-peer/v6-local, got %q", got)
	}
}

func TestSplitCSV(t *testing.T) {
	cases := map[string][]string{
		"":             nil,
		"  ":           nil,
		"a":            {"a"},
		"a,b,c":        {"a", "b", "c"},
		" a , b ,, c ": {"a", "b", "c"},
	}
	for in, want := range cases {
		got := splitCSV(in)
		if len(got) != len(want) {
			t.Errorf("splitCSV(%q)=%v len want %v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("splitCSV(%q)[%d]=%q want %q", in, i, got[i], want[i])
			}
		}
	}
}

func TestLocalIPVersion(t *testing.T) {
	// LocalIPVersion is observably one of 4/6/46 in any test environment.
	switch LocalIPVersion() {
	case "4", "6", "46":
	default:
		t.Errorf("LocalIPVersion() = %q, want 4/6/46", LocalIPVersion())
	}
}

func TestLocalSourceIPsCSV(t *testing.T) {
	s := LocalSourceIPsCSV()
	// Returns comma-separated string; may be empty on restricted sandbox CI but the
	// field should always be returned without panic.
	_ = strings.Split(s, ",")
}

// Ensure context import is used (compile check for the test-only sender alias).
var _ = context.Canceled
var _ = usecase.SendConfig{}
