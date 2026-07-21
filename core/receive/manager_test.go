package receive

import (
	"testing"
	"time"
)

func TestExpireExpired_PendingExpired(t *testing.T) {
	t.Parallel()
	m := NewManager(3)
	inc, _ := m.Register("peer-1", "Peer", "file.bin", 100, 30*time.Second)

	// Simulate time passing beyond expiry.
	m.ExpireExpired(time.Now().Add(35 * time.Second))

	got, err := m.Get(inc.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusExpired {
		t.Fatalf("status = %s, want expired", got.Status)
	}
}

func TestExpireExpired_NotYetExpired(t *testing.T) {
	t.Parallel()
	m := NewManager(3)
	inc, _ := m.Register("peer-1", "Peer", "file.bin", 100, 30*time.Second)

	m.ExpireExpired(time.Now().Add(10 * time.Second))

	got, _ := m.Get(inc.ID)
	if got.Status != StatusPending {
		t.Fatalf("status = %s, want pending", got.Status)
	}
}

func TestExpireExpired_ReceivingNotExpired(t *testing.T) {
	t.Parallel()
	m := NewManager(3)
	inc, _ := m.Register("peer-1", "Peer", "file.bin", 100, 30*time.Second)

	// Transition to receiving (simulating user accepted).
	m.Accept(inc.ID, "/tmp/save.bin")
	m.UpdateStatus(inc.ID, StatusReceiving, "")

	// Even after expiry time, receiving transfers should not auto-expire.
	m.ExpireExpired(time.Now().Add(60 * time.Second))

	got, _ := m.Get(inc.ID)
	if got.Status != StatusReceiving {
		t.Fatalf("status = %s, want receiving", got.Status)
	}
}

func TestExpireExpired_MultipleExpired(t *testing.T) {
	t.Parallel()
	m := NewManager(10)
	inc1, _ := m.Register("peer-1", "Peer1", "a.bin", 100, 30*time.Second)
	inc2, _ := m.Register("peer-2", "Peer2", "b.bin", 200, 30*time.Second)
	inc3, _ := m.Register("peer-3", "Peer3", "c.bin", 300, 60*time.Second) // longer expiry

	expired := m.ExpireExpired(time.Now().Add(35 * time.Second))

	if len(expired) != 2 {
		t.Fatalf("expired count = %d, want 2", len(expired))
	}

	got1, _ := m.Get(inc1.ID)
	got2, _ := m.Get(inc2.ID)
	got3, _ := m.Get(inc3.ID)
	if got1.Status != StatusExpired {
		t.Errorf("inc1 status = %s, want expired", got1.Status)
	}
	if got2.Status != StatusExpired {
		t.Errorf("inc2 status = %s, want expired", got2.Status)
	}
	if got3.Status != StatusPending {
		t.Errorf("inc3 status = %s, want pending", got3.Status)
	}
}

func TestExpireExpired_Idempotent(t *testing.T) {
	t.Parallel()
	m := NewManager(3)
	inc, _ := m.Register("peer-1", "Peer", "file.bin", 100, 30*time.Second)

	now := time.Now().Add(35 * time.Second)
	m.ExpireExpired(now)
	expired := m.ExpireExpired(now)

	if len(expired) != 0 {
		t.Fatalf("second ExpireExpired returned %d, want 0", len(expired))
	}

	got, _ := m.Get(inc.ID)
	if got.Status != StatusExpired {
		t.Fatalf("status = %s, want expired", got.Status)
	}
}

func TestDefaultPromptExpiry_Is30Seconds(t *testing.T) {
	if DefaultPromptExpiry != 30*time.Second {
		t.Fatalf("DefaultPromptExpiry = %v, want 30s (P1-23)", DefaultPromptExpiry)
	}
}
