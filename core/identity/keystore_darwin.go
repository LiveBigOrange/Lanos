//go:build darwin

package identity

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
)

// keychainKeystore stores the PEM data as a generic password item in the
// user's macOS Keychain (default: login keychain). The file path argument
// is ignored; the Keychain service/account pair uniquely identifies the
// identity. See PRD §4.3.
//
// We use the `security` CLI (pre-installed on macOS) rather than CGO
// bindings, to keep the build CGO-free.
type keychainKeystore struct{}

// platformKeystore returns the Keychain-backed keystore on macOS.
func platformKeystore() Keystore { return &keychainKeystore{} }

const (
	kcService = "Lanos"
	kcAccount = "identity-key"
)

func (keychainKeystore) Save(path string, pemData []byte) error {
	// -U updates the item if it already exists (idempotent re-saves).
	cmd := exec.Command("security", "add-generic-password",
		"-a", kcAccount, "-s", kcService,
		"-w", string(pemData), "-U")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain save: %w: %s", err, out)
	}
	return nil
}

func (keychainKeystore) Load(path string) ([]byte, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-a", kcAccount, "-s", kcService, "-w")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// `security find-generic-password` exits with code 44 when
			// the item is not found. We translate that to ErrNotFound so
			// LoadOrCreate knows to generate a new key.
			if exitErr.ExitCode() == 44 {
				return nil, ErrNotFound
			}
		}
		return nil, fmt.Errorf("keychain load: %w: %s", err, out)
	}
	// `security ... -w` prints the password followed by a single trailing
	// newline. Strip exactly one so PEM data (which itself ends in \n) is
	// preserved byte-for-byte.
	return bytes.TrimSuffix(out, []byte{'\n'}), nil
}
