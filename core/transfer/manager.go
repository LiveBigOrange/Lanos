package transfer

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("transfer: not found")
	ErrLimitReached  = errors.New("transfer: concurrent transfer limit reached")
	ErrAlreadyExists = errors.New("transfer: already exists for this file")
)

type Status string

const (
	StatusPending        Status = "pending"
	StatusConnecting     Status = "connecting"
	StatusTransferring   Status = "transferring"
	StatusCompleted      Status = "completed"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
	StatusAwaitingResume Status = "awaiting_resume"
)

type Transfer struct {
	ID        string    `json:"id"`
	PeerID    string    `json:"peer_id"`
	PeerName  string    `json:"peer_name"`
	FileName  string    `json:"file_name"`
	FilePath  string    `json:"file_path"`
	FileSize  int64     `json:"file_size"`
	SentBytes int64     `json:"sent_bytes"`
	Status    Status    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// sm enforces valid state transitions. Not serialized.
	sm *StateMachine `json:"-"`
}

func newTransfer(peerID, peerName, filePath, fileName string, fileSize int64) *Transfer {
	now := time.Now()
	return &Transfer{
		ID:        uuid.NewString(),
		PeerID:    peerID,
		PeerName:  peerName,
		FileName:  fileName,
		FilePath:  filePath,
		FileSize:  fileSize,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
		sm:        NewStateMachine(StatusPending),
	}
}

const DefaultMaxConcurrent = 3

type Manager struct {
	mu        sync.RWMutex
	transfers map[string]*Transfer
	maxActive int
	CancelReg *CancelRegistry
}

func NewManager(maxActive int) *Manager {
	if maxActive <= 0 {
		maxActive = DefaultMaxConcurrent
	}
	return &Manager{
		transfers: make(map[string]*Transfer),
		maxActive: maxActive,
		CancelReg: NewCancelRegistry(),
	}
}

func (m *Manager) Create(peerID, peerName, filePath, fileName string, fileSize int64) (*Transfer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	active := 0
	for _, t := range m.transfers {
		if t.Status == StatusPending || t.Status == StatusConnecting || t.Status == StatusTransferring {
			active++
		}
	}
	if active >= m.maxActive {
		return nil, ErrLimitReached
	}

	t := newTransfer(peerID, peerName, filePath, fileName, fileSize)
	m.transfers[t.ID] = t
	slog.Info("transfer created", "id", t.ID[:8], "peer", peerName, "file", fileName)
	return t, nil
}

func (m *Manager) Get(id string) (*Transfer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.transfers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (m *Manager) List() []*Transfer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Transfer, 0, len(m.transfers))
	for _, t := range m.transfers {
		out = append(out, t)
	}
	return out
}

func (m *Manager) UpdateStatus(id string, status Status, errMsg string) (*Transfer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.transfers[id]
	if !ok {
		return nil, ErrNotFound
	}
	if status != t.Status {
		if err := t.sm.Transition(status); err != nil {
			return nil, err
		}
		t.Status = status
	}
	t.UpdatedAt = time.Now()
	if errMsg != "" {
		t.Error = errMsg
	}
	return t, nil
}

func (m *Manager) UpdateProgress(id string, sentBytes int64) (*Transfer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.transfers[id]
	if !ok {
		return nil, ErrNotFound
	}
	t.SentBytes = sentBytes
	t.UpdatedAt = time.Now()
	return t, nil
}

func (m *Manager) Remove(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.transfers[id]; ok {
		delete(m.transfers, id)
		return true
	}
	return false
}

// ActiveCount returns the number of transfers currently occupying a slot in
// the max-concurrency budget. This MUST mirror the active-set used by Create,
// otherwise Create could admit more transfers than ActiveCount reports, or
// vice versa. Both functions use the same set: pending / connecting /
// transferring. StatusAwaitingResume is a quiescent state and does not count
// against the budget (a paused-but-resumable transfer is not actively
// consuming a connection).
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, t := range m.transfers {
		switch t.Status {
		case StatusPending, StatusConnecting, StatusTransferring:
			count++
		}
	}
	return count
}

func (m *Manager) Cancel(id string) (*Transfer, error) {
	m.mu.Lock()
	t, ok := m.transfers[id]
	if !ok {
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	if err := t.sm.Transition(StatusCancelled); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("transfer: cancel: %w", err)
	}
	t.Status = StatusCancelled
	t.UpdatedAt = time.Now()
	m.mu.Unlock()

	m.CancelReg.Cancel(id)

	slog.Info("transfer cancelled", "id", id[:8])
	return t, nil
}
