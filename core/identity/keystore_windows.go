//go:build windows

package identity

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// dpapiKeystore encrypts the PEM data with Windows DPAPI (CryptProtectData)
// before writing it to disk. The encrypted blob is tied to the current
// user's credentials, so it cannot be decrypted by another user or on
// another machine. See PRD §4.3.
type dpapiKeystore struct{}

// platformKeystore returns the DPAPI-backed keystore on Windows.
func platformKeystore() Keystore { return &dpapiKeystore{} }

var (
	modCrypt32             = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData   = modCrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = modCrypt32.NewProc("CryptUnprotectData")
	modKernel32            = syscall.NewLazyDLL("kernel32.dll")
	procLocalFree          = modKernel32.NewProc("LocalFree")
)

// dataBlob mirrors the Windows DATA_BLOB struct used by DPAPI.
type dataBlob struct {
	Size uint32
	Data *byte
}

func (dpapiKeystore) Save(path string, pemData []byte) error {
	enc, err := dpapiProtect(pemData)
	if err != nil {
		return fmt.Errorf("dpapi encrypt: %w", err)
	}
	return os.WriteFile(path, enc, 0o600)
}

func (dpapiKeystore) Load(path string) ([]byte, error) {
	enc, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out, err := dpapiUnprotect(enc)
	if err != nil {
		return nil, fmt.Errorf("dpapi decrypt: %w", err)
	}
	return out, nil
}

func dpapiProtect(data []byte) ([]byte, error) {
	var in dataBlob
	if len(data) > 0 {
		in.Size = uint32(len(data))
		in.Data = &data[0]
	}
	var out dataBlob
	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer localFree(out.Data)
	return blobToBytes(out), nil
}

func dpapiUnprotect(data []byte) ([]byte, error) {
	var in dataBlob
	if len(data) > 0 {
		in.Size = uint32(len(data))
		in.Data = &data[0]
	}
	var out dataBlob
	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer localFree(out.Data)
	return blobToBytes(out), nil
}

func localFree(p *byte) {
	if p != nil {
		procLocalFree.Call(uintptr(unsafe.Pointer(p)))
	}
}

func blobToBytes(b dataBlob) []byte {
	if b.Data == nil || b.Size == 0 {
		return nil
	}
	out := make([]byte, b.Size)
	copy(out, unsafe.Slice(b.Data, b.Size))
	return out
}
