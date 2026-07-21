package identity

import "errors"

// ErrNotFound is returned by Keystore.Load when no identity has been stored
// at the given path. Callers use this to decide whether to generate a new key.
var ErrNotFound = errors.New("identity not found")

// Keystore abstracts platform-specific at-rest protection of the identity
// private key. See PRD §4.3:
//   - Linux: plain file with 0600 permission
//   - Windows: DPAPI-encrypted blob in file
//   - macOS: macOS Keychain (generic password item)
//
// The data passed to Save/Load is the PEM-encoded identity (see
// pem.EncodeToMemory with block type "LANOS IDENTITY"). Each implementation
// is free to encrypt or relocate the data as appropriate for its platform.
type Keystore interface {
	// Save writes the PEM-encoded identity data. The path is the canonical
	// location; some implementations (macOS Keychain) may ignore it and
	// store elsewhere.
	Save(path string, pemData []byte) error
	// Load reads the PEM-encoded identity data. Returns ErrNotFound if no
	// identity has been stored at the given path.
	Load(path string) ([]byte, error)
}

// platformKeystore returns the Keystore appropriate for the host OS.
// Implemented per-platform in keystore_plain.go / keystore_windows.go /
// keystore_darwin.go via build tags.
