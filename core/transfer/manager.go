package transfer

import (
	"errors"
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
	StatusPending     Status = "pending"
	StatusConnecting  Status = "connecting"
	StatusTransferring Status = "transferring"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
)

type Transfer struct {
	ID         string    `json:"id"`
	PeerID     string    `json:"peer_id"`
	PeerName   string    `json:"peer_name"`
	FileName   string    `json:"file_name"`
	FilePath   string    `json:"file_path"`
	FileSize   int64     `json:"file_size"`
	SentBytes  int64     `json:"sent_bytes"`
	Status     Status    `json:"status"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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
	}
}

const DefaultMaxConcurrent = 3

type Manager struct {
	mu        sync.RWMutex
	transfers map[string]*Transfer
	maxActive int
}

func NewManager(maxActive int) *Manager {
	if maxActive <= 0 {
		maxActive = DefaultMaxConcurrent
	}
	return &Manager{
		transfers: make(map[string]*Transfer),
		maxActive: maxActive,
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
	t.Status = status
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
	defer m.mu.Unlock()
	t, ok := m.transfers[id]
	if !ok {
		return nil, ErrNotFound
	}
	if t.Status == StatusCompleted {
		return nil, errors.New("transfer: cannot cancel completed transfer")
	}
	t.Status = StatusCancelled
	t.UpdatedAt = time.Now()
	slog.Info("transfer cancelled", "id", id[:8])
	return t, nil
}
