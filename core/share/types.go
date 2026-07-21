// Package share implements the web share HTTP server for Lanos.
// It allows sharing files/folders via browser-downloadable links with
// optional password protection, download limits, and expiry.
package share

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"time"
)

// TokenLength is the raw byte length of a share token (256 bits).
const TokenLength = 32

// TokenURLLength is the base64url-encoded length of a token.
const TokenURLLength = 43 // ceil(32/3)*4 without padding

// MaxShares is the default maximum number of concurrent active shares.
const MaxShares = 64

// MaxDownloadCount is the default maximum number of downloads per share.
const MaxDownloadCount = 10

// DefaultExpiry is the default share expiry duration.
const DefaultExpiry = 30 * time.Minute

// PasswordMinLength is the minimum password length per PRD §3.3.8.
const PasswordMinLength = 4

// PasswordMaxLength is the maximum password length per PRD §3.3.8.
const PasswordMaxLength = 32

// MaxPasswordAttempts is the number of failed password attempts before
// an IP is temporarily banned.
const MaxPasswordAttempts = 10

// MaxTokenAttempts is the number of failed token lookups before an IP
// is temporarily banned.
const MaxTokenAttempts = 10

// BanDuration is how long an IP is banned after exceeding attempt limits.
const BanDuration = 5 * time.Minute

// ZipStreamTimeout is the maximum duration for a single ZIP stream download.
const ZipStreamTimeout = 30 * time.Minute

// Share represents a single active web share.
type Share struct {
	Token        string    `json:"token"`
	Path         string    `json:"path"` // absolute file/folder path
	IsDir        bool      `json:"is_dir"`
	Name         string    `json:"name"` // display name (base name of path)
	Size         int64     `json:"size"` // total size in bytes (0 for dirs until zipped)
	PasswordHash [32]byte  `json:"-"`    // SHA256(password + salt)
	Salt         [16]byte  `json:"-"`    // per-share random salt
	HasPassword  bool      `json:"has_password"`
	Expiry       time.Time `json:"expiry"`
	MaxDownloads int       `json:"max_downloads"`
	Downloads    int       `json:"downloads"`
	CreatedAt    time.Time `json:"created_at"`
	Stopped      bool      `json:"stopped"`
}

// NewShare creates a new share with a random token and optional password.
func NewShare(path string, isDir bool, name string, size int64, password string, expiry time.Duration, maxDownloads int) (*Share, error) {
	token, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("share: generate token: %w", err)
	}

	s := &Share{
		Token:        token,
		Path:         path,
		IsDir:        isDir,
		Name:         name,
		Size:         size,
		Expiry:       time.Now().Add(expiry),
		MaxDownloads: maxDownloads,
		CreatedAt:    time.Now(),
	}

	if password != "" {
		if len(password) < PasswordMinLength || len(password) > PasswordMaxLength {
			return nil, fmt.Errorf("share: password length must be %d-%d chars", PasswordMinLength, PasswordMaxLength)
		}
		if _, err := rand.Read(s.Salt[:]); err != nil {
			return nil, fmt.Errorf("share: generate salt: %w", err)
		}
		s.PasswordHash = HashPassword(password, s.Salt)
		s.HasPassword = true
	}

	return s, nil
}

// GenerateToken creates a cryptographically random 32-byte token encoded
// as URL-safe base64 (43 chars, no padding).
func GenerateToken() (string, error) {
	var b [TokenLength]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("share: rand token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// HashPassword computes SHA256(password + salt).
func HashPassword(password string, salt [16]byte) [32]byte {
	h := sha256.New()
	h.Write([]byte(password))
	h.Write(salt[:])
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// VerifyPassword checks if the given password matches the stored hash.
// Comparison is constant-time to avoid leaking information about the hash
// to an attacker who can time password-submission responses.
func (s *Share) VerifyPassword(password string) bool {
	if !s.HasPassword {
		return true
	}
	got := HashPassword(password, s.Salt)
	return subtle.ConstantTimeCompare(s.PasswordHash[:], got[:]) == 1
}

// Expired reports whether the share has passed its expiry time.
func (s *Share) Expired() bool {
	return time.Now().After(s.Expiry)
}

// Exhausted reports whether the share has reached its download limit.
func (s *Share) Exhausted() bool {
	return s.MaxDownloads > 0 && s.Downloads >= s.MaxDownloads
}

// Active reports whether the share is still valid (not stopped, expired, or exhausted).
func (s *Share) Active() bool {
	return !s.Stopped && !s.Expired() && !s.Exhausted()
}

// RemainingDownloads returns the number of downloads left, or -1 for unlimited.
func (s *Share) RemainingDownloads() int {
	if s.MaxDownloads <= 0 {
		return -1
	}
	r := s.MaxDownloads - s.Downloads
	if r < 0 {
		return 0
	}
	return r
}

// RemainingTime returns the duration until expiry, or 0 if expired.
func (s *Share) RemainingTime() time.Duration {
	d := time.Until(s.Expiry)
	if d < 0 {
		return 0
	}
	return d
}
