// Package instance implements cross-platform single-instance locking.
// See PRD §4.4 单实例控制.
//
// Lock file location: $TMPDIR/lanos.lock (or /tmp on Unix, %TEMP% on Windows).
// The lock is held for the lifetime of the gcd process. If another instance
// is already running, Acquire returns ErrAlreadyRunning.
//
// Implementation:
//   - Unix (linux/darwin/...): flock(2) with LOCK_EX | LOCK_NB. The lock
//     is automatically released when the process exits, even on crash.
//   - Windows: LockFileEx with LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY.
//     The lock is released when the file handle is closed (process exit closes
//     all handles), so it is also crash-safe.
package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrAlreadyRunning is returned when another gcd instance holds the lock.
var ErrAlreadyRunning = errors.New("lanos already running")

// Lock represents an acquired single-instance lock. Call Release on shutdown.
type Lock struct {
	path   string
	handle *os.File
}

// Acquire attempts to take the single-instance lock. On success returns a
// Lock the caller must Release. On failure returns ErrAlreadyRunning.
func Acquire() (*Lock, error) {
	dir := os.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ensure temp dir: %w", err)
	}
	path := filepath.Join(dir, "lanos.lock")

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}

	// Platform-specific exclusive non-blocking lock (flock on Unix,
	// LockFileEx on Windows). Both surface a non-nil error when the lock
	// is already held by another process / handle.
	if err := flockTry(f); err != nil {
		f.Close()
		return nil, ErrAlreadyRunning
	}
	return &Lock{path: path, handle: f}, nil
}

// Release drops the lock and closes the file. Safe to call multiple times;
// subsequent calls are no-ops.
func (l *Lock) Release() error {
	if l == nil || l.handle == nil {
		return nil
	}
	flockUnlock(l.handle)
	err := l.handle.Close()
	l.handle = nil
	_ = os.Remove(l.path)
	return err
}
