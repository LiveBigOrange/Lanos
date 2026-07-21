package share

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// ErrShareNotFound is returned when a token does not match any active share.
var ErrShareNotFound = errors.New("share: not found")

// ErrShareLimit is returned when the maximum number of concurrent shares is reached.
var ErrShareLimit = errors.New("share: concurrent share limit reached")

// ErrIPBanned is returned when an IP has exceeded attempt limits.
var ErrIPBanned = errors.New("share: IP temporarily banned")

// Manager tracks active shares, enforces limits, and handles IP banning.
type Manager struct {
	mu       sync.RWMutex
	shares   map[string]*Share // token -> share
	maxShares int

	// IP ban tracking
	banMu      sync.Mutex
	tokenFails map[string]int // IP -> failed token attempts
	passFails  map[string]int // IP -> failed password attempts
	bannedUntil map[string]time.Time // IP -> ban expiry
}

// NewManager creates a share manager with the given concurrency limit.
func NewManager(maxShares int) *Manager {
	if maxShares <= 0 {
		maxShares = MaxShares
	}
	m := &Manager{
		shares:      make(map[string]*Share),
		maxShares:   maxShares,
		tokenFails:  make(map[string]int),
		passFails:   make(map[string]int),
		bannedUntil: make(map[string]time.Time),
	}
	// Background cleanup goroutine
	go m.cleanupLoop()
	return m
}

// CreateShare registers a new share. Returns ErrShareLimit if at capacity.
func (m *Manager) CreateShare(path string, isDir bool, name string, size int64, password string, expiry time.Duration, maxDownloads int) (*Share, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.shares) >= m.maxShares {
		return nil, ErrShareLimit
	}

	s, err := NewShare(path, isDir, name, size, password, expiry, maxDownloads)
	if err != nil {
		return nil, err
	}
	m.shares[s.Token] = s
	slog.Info("share created", "token", s.Token[:8]+"...", "name", name, "expiry", expiry, "maxDownloads", maxDownloads)
	return s, nil
}

// GetShare retrieves a share by token. Returns ErrShareNotFound if missing
// or inactive. Increments the failure counter for the given IP on miss.
func (m *Manager) GetShare(token, ip string) (*Share, error) {
	m.mu.RLock()
	s, ok := m.shares[token]
	m.mu.RUnlock()

	if !ok || !s.Active() {
		m.recordTokenFail(ip)
		return nil, ErrShareNotFound
	}
	m.clearTokenFails(ip)
	return s, nil
}

// StopShare marks a share as stopped and removes it from the active set.
func (m *Manager) StopShare(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.shares[token]; ok {
		s.Stopped = true
		delete(m.shares, token)
		slog.Info("share stopped", "token", token[:8]+"...")
		return true
	}
	return false
}

// StopAll stops all active shares (called on shutdown).
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for token, s := range m.shares {
		s.Stopped = true
		delete(m.shares, token)
	}
	slog.Info("all shares stopped", "count", len(m.shares))
}

// ListShares returns a snapshot of all active shares.
func (m *Manager) ListShares() []*Share {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Share, 0, len(m.shares))
	for _, s := range m.shares {
		if s.Active() {
			out = append(out, s)
		}
	}
	return out
}

// ActiveCount returns the number of currently active shares.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, s := range m.shares {
		if s.Active() {
			count++
		}
	}
	return count
}

// RecordDownload increments the download counter for a share.
func (m *Manager) RecordDownload(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.shares[token]; ok {
		s.Downloads++
	}
}

// CheckPassword verifies the password for a share and tracks failures by IP.
func (m *Manager) CheckPassword(s *Share, password, ip string) error {
	if err := m.checkBanned(ip); err != nil {
		return err
	}
	if !s.VerifyPassword(password) {
		m.recordPassFail(ip)
		return fmt.Errorf("share: invalid password")
	}
	m.clearPassFails(ip)
	return nil
}

// --- IP banning ---

func (m *Manager) checkBanned(ip string) error {
	m.banMu.Lock()
	defer m.banMu.Unlock()
	if until, banned := m.bannedUntil[ip]; banned {
		if time.Now().Before(until) {
			return ErrIPBanned
		}
		// Ban expired, clear it
		delete(m.bannedUntil, ip)
		delete(m.tokenFails, ip)
		delete(m.passFails, ip)
	}
	return nil
}

func (m *Manager) recordTokenFail(ip string) {
	m.banMu.Lock()
	defer m.banMu.Unlock()
	m.tokenFails[ip]++
	if m.tokenFails[ip] >= MaxTokenAttempts {
		m.bannedUntil[ip] = time.Now().Add(BanDuration)
		slog.Warn("IP banned for token enumeration", "ip", ip, "attempts", m.tokenFails[ip])
		delete(m.tokenFails, ip)
	}
}

func (m *Manager) recordPassFail(ip string) {
	m.banMu.Lock()
	defer m.banMu.Unlock()
	m.passFails[ip]++
	if m.passFails[ip] >= MaxPasswordAttempts {
		m.bannedUntil[ip] = time.Now().Add(BanDuration)
		slog.Warn("IP banned for password brute force", "ip", ip, "attempts", m.passFails[ip])
		delete(m.passFails, ip)
	}
}

func (m *Manager) clearTokenFails(ip string) {
	m.banMu.Lock()
	defer m.banMu.Unlock()
	delete(m.tokenFails, ip)
}

func (m *Manager) clearPassFails(ip string) {
	m.banMu.Lock()
	defer m.banMu.Unlock()
	delete(m.passFails, ip)
}

// --- Background cleanup ---

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.cleanup()
	}
}

func (m *Manager) cleanup() {
	m.mu.Lock()
	now := time.Now()
	for token, s := range m.shares {
		if s.Stopped || now.After(s.Expiry) || s.Exhausted() {
			delete(m.shares, token)
		}
	}
	m.mu.Unlock()

	// Clean expired bans
	m.banMu.Lock()
	for ip, until := range m.bannedUntil {
		if now.After(until) {
			delete(m.bannedUntil, ip)
			delete(m.tokenFails, ip)
			delete(m.passFails, ip)
		}
	}
	m.banMu.Unlock()
}

// --- Path validation ---

// ValidateToken checks that a token is well-formed (43 chars, URL-safe base64)
// and does not contain path traversal attempts.
func ValidateToken(token string) error {
	if len(token) != TokenURLLength {
		return fmt.Errorf("share: invalid token length %d", len(token))
	}
	for _, c := range token {
		if !isURLSafeBase64(c) {
			return fmt.Errorf("share: invalid token character %q", c)
		}
	}
	return nil
}

func isURLSafeBase64(c rune) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
}

// ClientIP extracts the client IP from a request, handling X-Forwarded-For
// and X-Real-IP headers (though MVP is direct LAN, no proxy).
func ClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
