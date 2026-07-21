package receive

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound     = errors.New("receive: not found")
	ErrLimitReached = errors.New("receive: concurrent receive limit reached")
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusPrompting  Status = "prompting"
	StatusAccepting  Status = "accepting"
	StatusReceiving  Status = "receiving"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusRejected   Status = "rejected"
	StatusCancelled  Status = "cancelled"
	StatusExpired    Status = "expired"
)

type Incoming struct {
	ID         string    `json:"id"`
	PeerID     string    `json:"peer_id"`
	PeerName   string    `json:"peer_name"`
	FileName   string    `json:"file_name"`
	FileSize   int64     `json:"file_size"`
	SavePath   string    `json:"save_path"`
	ReceivedBytes int64  `json:"received_bytes"`
	Status     Status    `json:"status"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func newIncoming(peerID, peerName, fileName string, fileSize int64, expiry time.Duration) *Incoming {
	now := time.Now()
	return &Incoming{
		ID:        uuid.NewString(),
		PeerID:    peerID,
		PeerName:  peerName,
		FileName:  fileName,
		FileSize:  fileSize,
		Status:    StatusPending,
		CreatedAt: now,
		ExpiresAt: now.Add(expiry),
		UpdatedAt: now,
	}
}

const (
	DefaultMaxConcurrent = 3
	DefaultPromptExpiry  = 2 * time.Minute
)

type Manager struct {
	mu        sync.RWMutex
	incomings map[string]*Incoming
	maxActive int
}

func NewManager(maxActive int) *Manager {
	if maxActive <= 0 {
		maxActive = DefaultMaxConcurrent
	}
	return &Manager{
		incomings: make(map[string]*Incoming),
		maxActive: maxActive,
	}
}

func (m *Manager) Register(peerID, peerName, fileName string, fileSize int64, expiry time.Duration) (*Incoming, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	active := 0
	for _, inc := range m.incomings {
		if inc.Status == StatusPrompting || inc.Status == StatusAccepting || inc.Status == StatusReceiving {
			active++
		}
	}
	if active >= m.maxActive {
		return nil, ErrLimitReached
	}

	if expiry <= 0 {
		expiry = DefaultPromptExpiry
	}
	inc := newIncoming(peerID, peerName, fileName, fileSize, expiry)
	m.incomings[inc.ID] = inc
	slog.Info("incoming transfer registered", "id", inc.ID[:8], "peer", peerName, "file", fileName)
	return inc, nil
}

func (m *Manager) Get(id string) (*Incoming, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inc, ok := m.incomings[id]
	if !ok {
		return nil, ErrNotFound
	}
	return inc, nil
}

func (m *Manager) List() []*Incoming {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Incoming, 0, len(m.incomings))
	for _, inc := range m.incomings {
		out = append(out, inc)
	}
	return out
}

func (m *Manager) Accept(id, savePath string) (*Incoming, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inc, ok := m.incomings[id]
	if !ok {
		return nil, ErrNotFound
	}
	if inc.Status != StatusPending && inc.Status != StatusPrompting {
		return nil, errors.New("receive: cannot accept in current state")
	}
	inc.Status = StatusAccepting
	inc.SavePath = savePath
	inc.UpdatedAt = time.Now()
	slog.Info("incoming transfer accepted", "id", id[:8], "savePath", savePath)
	return inc, nil
}

func (m *Manager) Reject(id string) (*Incoming, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inc, ok := m.incomings[id]
	if !ok {
		return nil, ErrNotFound
	}
	if inc.Status == StatusCompleted || inc.Status == StatusReceiving {
		return nil, errors.New("receive: cannot reject in current state")
	}
	inc.Status = StatusRejected
	inc.UpdatedAt = time.Now()
	slog.Info("incoming transfer rejected", "id", id[:8])
	return inc, nil
}

func (m *Manager) UpdateStatus(id string, status Status, errMsg string) (*Incoming, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inc, ok := m.incomings[id]
	if !ok {
		return nil, ErrNotFound
	}
	inc.Status = status
	inc.UpdatedAt = time.Now()
	if errMsg != "" {
		inc.Error = errMsg
	}
	return inc, nil
}

func (m *Manager) UpdateProgress(id string, receivedBytes int64) (*Incoming, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inc, ok := m.incomings[id]
	if !ok {
		return nil, ErrNotFound
	}
	inc.ReceivedBytes = receivedBytes
	inc.UpdatedAt = time.Now()
	return inc, nil
}

func (m *Manager) Remove(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.incomings[id]; ok {
		delete(m.incomings, id)
		return true
	}
	return false
}

func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, inc := range m.incomings {
		switch inc.Status {
		case StatusPending, StatusPrompting, StatusAccepting, StatusReceiving:
			count++
		}
	}
	return count
}

func (m *Manager) Cancel(id string) (*Incoming, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inc, ok := m.incomings[id]
	if !ok {
		return nil, ErrNotFound
	}
	if inc.Status == StatusCompleted {
		return nil, errors.New("receive: cannot cancel completed transfer")
	}
	inc.Status = StatusCancelled
	inc.UpdatedAt = time.Now()
	slog.Info("incoming transfer cancelled", "id", id[:8])
	return inc, nil
}
