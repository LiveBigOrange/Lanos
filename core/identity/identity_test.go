package identity

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempHome redirects $HOME (and APPDATA on Windows) so tests don't
// touch the real user data dir.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", filepath.Join(dir, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	return dir
}

func TestLoadOrCreateGenerates(t *testing.T) {
	withTempHome(t)

	id, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if len(id.PrivED) != ed25519.PrivateKeySize {
		t.Errorf("priv key size = %d, want %d", len(id.PrivED), ed25519.PrivateKeySize)
	}
	if len(id.PubED) != ed25519.PublicKeySize {
		t.Errorf("pub key size = %d, want %d", len(id.PubED), ed25519.PublicKeySize)
	}
	if len(id.PubHash) != 32 { // 16 bytes hex = 32 chars
		t.Errorf("PubHash length = %d, want 32", len(id.PubHash))
	}
	if len(id.DeviceID) != 16 { // 8 bytes hex = 16 chars
		t.Errorf("DeviceID length = %d, want 16", len(id.DeviceID))
	}
	if !strings.HasPrefix(id.PubHash, id.DeviceID) {
		t.Errorf("DeviceID %q must be prefix of PubHash %q", id.DeviceID, id.PubHash)
	}
}

func TestLoadOrCreateIsPersistent(t *testing.T) {
	withTempHome(t)

	id1, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("first LoadOrCreate: %v", err)
	}
	id2, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("second LoadOrCreate: %v", err)
	}
	if string(id1.PrivED) != string(id2.PrivED) {
		t.Errorf("identity not persisted: keys differ")
	}
}

func TestLoadOrCreateFilePermission(t *testing.T) {
	withTempHome(t)

	id, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	_ = id

	// Locate the key file; we don't expose its path publicly so re-derive it.
	// On non-Windows the file must have 0600.
	if runtimeGOOS() == "windows" {
		t.Skip("permission check is unix-only")
	}
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "lanos", "identity.key")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat identity file: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("identity file mode = %o, want 0600", mode)
	}
}
