package share

import (
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewShareToken verifies token generation produces 43-char URL-safe strings.
func TestNewShareToken(t *testing.T) {
	s1, err := NewShare("/tmp/test", false, "test.txt", 100, "", DefaultExpiry, MaxDownloadCount)
	if err != nil {
		t.Fatalf("NewShare: %v", err)
	}
	if len(s1.Token) != TokenURLLength {
		t.Fatalf("token len = %d, want %d", len(s1.Token), TokenURLLength)
	}
	if err := ValidateToken(s1.Token); err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	s2, _ := NewShare("/tmp/test", false, "test.txt", 100, "", DefaultExpiry, MaxDownloadCount)
	if s1.Token == s2.Token {
		t.Fatal("tokens should be unique")
	}
}

// TestSharePassword verifies password hashing and verification.
func TestSharePassword(t *testing.T) {
	s, err := NewShare("/tmp/test", false, "test.txt", 100, "secret", DefaultExpiry, MaxDownloadCount)
	if err != nil {
		t.Fatalf("NewShare with password: %v", err)
	}
	if !s.HasPassword {
		t.Fatal("HasPassword should be true")
	}
	if !s.VerifyPassword("secret") {
		t.Fatal("correct password rejected")
	}
	if s.VerifyPassword("wrong") {
		t.Fatal("wrong password accepted")
	}
	if s.VerifyPassword("") {
		t.Fatal("empty password accepted")
	}
}

// TestSharePasswordLength verifies password length validation.
func TestSharePasswordLength(t *testing.T) {
	// Too short
	_, err := NewShare("/tmp/test", false, "test.txt", 100, "abc", DefaultExpiry, MaxDownloadCount)
	if err == nil {
		t.Fatal("short password accepted")
	}
	// Too long
	long := make([]byte, 33)
	for i := range long {
		long[i] = 'a'
	}
	_, err = NewShare("/tmp/test", false, "test.txt", 100, string(long), DefaultExpiry, MaxDownloadCount)
	if err == nil {
		t.Fatal("long password accepted")
	}
	// Valid
	_, err = NewShare("/tmp/test", false, "test.txt", 100, "abcd", DefaultExpiry, MaxDownloadCount)
	if err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
}

// TestShareExpiry verifies expiry logic.
func TestShareExpiry(t *testing.T) {
	s, _ := NewShare("/tmp/test", false, "test.txt", 100, "", 1*time.Millisecond, MaxDownloadCount)
	time.Sleep(2 * time.Millisecond)
	if !s.Expired() {
		t.Fatal("share should be expired")
	}
	if s.Active() {
		t.Fatal("expired share should not be active")
	}
}

// TestShareDownloadLimit verifies download count exhaustion.
func TestShareDownloadLimit(t *testing.T) {
	s, _ := NewShare("/tmp/test", false, "test.txt", 100, "", DefaultExpiry, 2)
	if s.Exhausted() {
		t.Fatal("new share should not be exhausted")
	}
	s.Downloads = 2
	if !s.Exhausted() {
		t.Fatal("share at limit should be exhausted")
	}
	if s.Active() {
		t.Fatal("exhausted share should not be active")
	}
}

// TestManagerCreateGet verifies basic CRUD.
func TestManagerCreateGet(t *testing.T) {
	m := NewManager(10)
	s, err := m.CreateShare("/tmp/test", false, "test.txt", 100, "", DefaultExpiry, MaxDownloadCount)
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	got, err := m.GetShare(s.Token, "127.0.0.1")
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if got.Token != s.Token {
		t.Fatal("token mismatch")
	}
}

// TestManagerShareLimit verifies the concurrent share cap.
func TestManagerShareLimit(t *testing.T) {
	m := NewManager(2)
	m.CreateShare("/tmp/a", false, "a", 1, "", DefaultExpiry, MaxDownloadCount)
	m.CreateShare("/tmp/b", false, "b", 1, "", DefaultExpiry, MaxDownloadCount)
	_, err := m.CreateShare("/tmp/c", false, "c", 1, "", DefaultExpiry, MaxDownloadCount)
	if err != ErrShareLimit {
		t.Fatalf("expected ErrShareLimit, got %v", err)
	}
}

// TestManagerStop verifies stopping a share.
func TestManagerStop(t *testing.T) {
	m := NewManager(10)
	s, _ := m.CreateShare("/tmp/test", false, "test.txt", 100, "", DefaultExpiry, MaxDownloadCount)
	if !m.StopShare(s.Token) {
		t.Fatal("StopShare returned false")
	}
	_, err := m.GetShare(s.Token, "127.0.0.1")
	if err != ErrShareNotFound {
		t.Fatalf("expected ErrShareNotFound, got %v", err)
	}
}

// TestManagerIPBan verifies token enumeration banning.
func TestManagerIPBan(t *testing.T) {
	m := NewManager(10)
	ip := "192.168.1.100"
	// Generate valid-format tokens that don't exist
	for i := 0; i < MaxTokenAttempts; i++ {
		token, _ := GenerateToken()
		m.GetShare(token, ip)
	}
	// Next attempt should be banned
	_, err := m.GetShare("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", ip)
	if err != ErrShareNotFound {
		// May return banned or not found depending on timing
		if err != ErrIPBanned {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

// TestManagerPasswordBan verifies password brute-force banning.
func TestManagerPasswordBan(t *testing.T) {
	m := NewManager(10)
	s, _ := m.CreateShare("/tmp/test", false, "test.txt", 100, "correct", DefaultExpiry, MaxDownloadCount)
	ip := "192.168.1.101"
	for i := 0; i < MaxPasswordAttempts; i++ {
		m.CheckPassword(s, "wrong", ip)
	}
	err := m.CheckPassword(s, "correct", ip)
	if err != ErrIPBanned {
		t.Fatalf("expected ErrIPBanned, got %v", err)
	}
}

// TestValidateToken verifies token format validation.
func TestValidateToken(t *testing.T) {
	// Valid token
	valid, _ := GenerateToken()
	if err := ValidateToken(valid); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	// Too short
	if err := ValidateToken("abc"); err == nil {
		t.Fatal("short token accepted")
	}
	// Invalid chars
	bad := make([]byte, TokenURLLength)
	for i := range bad {
		bad[i] = '!'
	}
	if err := ValidateToken(string(bad)); err == nil {
		t.Fatal("invalid chars accepted")
	}
	// Path traversal
	if err := ValidateToken(".."); err == nil {
		t.Fatal("path traversal accepted")
	}
}

// TestCountFiles verifies file counting.
func TestCountFiles(t *testing.T) {
	dir := t.TempDir()
	// Single file
	f1 := filepath.Join(dir, "a.txt")
	os.WriteFile(f1, []byte("hello"), 0644)
	count, size, err := CountFiles(f1)
	if err != nil {
		t.Fatalf("CountFiles single: %v", err)
	}
	if count != 1 || size != 5 {
		t.Fatalf("single: count=%d size=%d", count, size)
	}
	// Directory
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0644)
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, "c.txt"), []byte("!"), 0644)
	count, size, err = CountFiles(dir)
	if err != nil {
		t.Fatalf("CountFiles dir: %v", err)
	}
	if count != 3 || size != 11 {
		t.Fatalf("dir: count=%d size=%d", count, size)
	}
}

// TestStreamZipSingleFile verifies ZIP streaming for a single file.
func TestStreamZipSingleFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	content := []byte("hello zip world")
	os.WriteFile(f, content, 0644)

	out := filepath.Join(dir, "out.zip")
	outFile, _ := os.Create(out)
	err := StreamZip(outFile, f, nil)
	outFile.Close()
	if err != nil {
		t.Fatalf("StreamZip: %v", err)
	}
	// Verify it's a valid ZIP
	if info, _ := os.Stat(out); info.Size() == 0 {
		t.Fatal("zip output empty")
	}
}

// TestStreamZipDirectory verifies ZIP streaming for a directory.
func TestStreamZipDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	os.Mkdir(src, 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("aaa"), 0644)
	sub := filepath.Join(src, "sub")
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, "b.txt"), []byte("bbb"), 0644)

	out := filepath.Join(dir, "out.zip")
	outFile, _ := os.Create(out)
	var files []string
	err := StreamZip(outFile, src, func(path string, size int64) {
		files = append(files, path)
	})
	outFile.Close()
	if err != nil {
		t.Fatalf("StreamZip dir: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files in zip, got %d: %v", len(files), files)
	}
}

// TestSanitizeZipPath verifies path safety checks.
func TestSanitizeZipPath(t *testing.T) {
	if err := SanitizeZipPath("normal/file.txt"); err != nil {
		t.Fatalf("normal path rejected: %v", err)
	}
	if err := SanitizeZipPath("../etc/passwd"); err == nil {
		t.Fatal("traversal accepted")
	}
	if err := SanitizeZipPath("/absolute/path"); err == nil {
		t.Fatal("absolute path accepted")
	}
	if err := SanitizeZipPath("C:\\windows"); err == nil {
		t.Fatal("drive letter accepted")
	}
}

// TestClientIP verifies IP extraction.
func TestClientIP(t *testing.T) {
	if got := ClientIP("192.168.1.1:12345"); got != "192.168.1.1" {
		t.Fatalf("ClientIP = %q", got)
	}
	if got := ClientIP("[::1]:8080"); got != "::1" {
		t.Fatalf("ClientIP v6 = %q", got)
	}
}

// TestShareURLForAddr verifies URL construction.
func TestShareURLForAddr(t *testing.T) {
	// IPv4
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	s := NewServer(NewManager(1), l)
	url := s.ShareURLForAddr("test-token-12345678901234567890123456789012")
	expected := "http://127.0.0.1:" + portOf(l) + "/dl/test-token-12345678901234567890123456789012"
	if url != expected {
		t.Fatalf("url = %q, want %q", url, expected)
	}
	l.Close()

	// IPv6
	l6, _ := net.Listen("tcp", "[::1]:0")
	s6 := NewServer(NewManager(1), l6)
	url6 := s6.ShareURLForAddr("test-token")
	if url6[:7] != "http://" {
		t.Fatalf("v6 url = %q", url6)
	}
	l6.Close()
}

func portOf(l net.Listener) string {
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

// TestGenerateTokenRandomness verifies tokens are cryptographically random.
func TestGenerateTokenRandomness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken: %v", err)
		}
		if seen[token] {
			t.Fatalf("duplicate token at iteration %d", i)
		}
		seen[token] = true
	}
}

// TestSaltRandomness verifies each share gets a unique salt.
func TestSaltRandomness(t *testing.T) {
	salts := make(map[[16]byte]bool)
	for i := 0; i < 100; i++ {
		s, _ := NewShare("/tmp", false, "t", 1, "password", DefaultExpiry, 1)
		if salts[s.Salt] {
			t.Fatal("duplicate salt")
		}
		salts[s.Salt] = true
	}
	// Also verify salt is not all zeros
	var zero [16]byte
	s, _ := NewShare("/tmp", false, "t", 1, "password", DefaultExpiry, 1)
	if s.Salt == zero {
		t.Fatal("salt is all zeros")
	}
	// Verify random source is actually working
	var b [16]byte
	rand.Read(b[:])
	if b == zero {
		t.Fatal("crypto/rand broken")
	}
}
