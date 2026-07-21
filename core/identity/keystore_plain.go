//go:build !windows && !darwin

package identity

import (
	"errors"
	"os"
)

// plainFileKeystore stores the PEM data as a plain file with 0600 permission.
// This is the Linux/BSD fallback; the filesystem permissions are the only
// at-rest protection. See PRD §4.3.
type plainFileKeystore struct{}

// platformKeystore returns the plain-file keystore on Linux/BSD.
func platformKeystore() Keystore { return &plainFileKeystore{} }

func (plainFileKeystore) Save(path string, pemData []byte) error {
	return os.WriteFile(path, pemData, 0o600)
}

func (plainFileKeystore) Load(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}
