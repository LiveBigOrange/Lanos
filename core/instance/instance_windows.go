//go:build windows

package instance

import (
	"os"
	"syscall"
	"unsafe"
)

// Windows LockFileEx flags. These are not exported by the syscall package,
// so we define them here. Values from the Windows SDK (winbase.h).
const (
	winLockfileExclusiveLock   = 0x02
	winLockfileFailImmediately = 0x01
)

var (
	modKernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modKernel32.NewProc("LockFileEx")
	procUnlockFileEx = modKernel32.NewProc("UnlockFileEx")
)

// overlapped mirrors the Windows OVERLAPPED struct. We define our own
// (rather than using syscall.Overlapped) to keep the layout explicit.
type overlapped struct {
	internal     uintptr
	internalHigh uintptr
	offset       uintptr
	offsetHigh   uintptr
	hEvent       uintptr
}

// flockTry acquires an exclusive, non-blocking byte-range lock via LockFileEx.
// On Windows, LockFileEx locks are associated with the file handle; a second
// OpenFile on the same path yields a different handle and the lock attempt
// fails with ERROR_LOCK_VIOLATION (33), which we surface as a non-nil error.
//
// Unlike POSIX flock, LockFileEx locks are NOT automatically released when
// the process exits - they are released when the last handle to the file is
// closed. Our Release() closes the handle, and process exit also closes all
// handles, so this is safe for the single-instance use case.
func flockTry(f *os.File) error {
	const reserved uint32 = 0
	const numBytesLow uint32 = 1
	const numBytesHigh uint32 = 0
	var ol overlapped
	handle := syscall.Handle(f.Fd())
	r, _, err := procLockFileEx.Call(
		uintptr(handle),
		uintptr(winLockfileExclusiveLock|winLockfileFailImmediately),
		uintptr(reserved),
		uintptr(numBytesLow),
		uintptr(numBytesHigh),
		uintptr(unsafe.Pointer(&ol)),
	)
	if r == 0 {
		return err // typically ERROR_LOCK_VIOLATION (33) when already held
	}
	return nil
}

// flockUnlock releases the byte-range lock acquired by flockTry.
func flockUnlock(f *os.File) error {
	const reserved uint32 = 0
	const numBytesLow uint32 = 1
	const numBytesHigh uint32 = 0
	var ol overlapped
	handle := syscall.Handle(f.Fd())
	r, _, err := procUnlockFileEx.Call(
		uintptr(handle),
		uintptr(reserved),
		uintptr(numBytesLow),
		uintptr(numBytesHigh),
		uintptr(unsafe.Pointer(&ol)),
	)
	if r == 0 {
		return err
	}
	return nil
}
