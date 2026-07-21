package lifecycle

import (
	"io"
	"os"
)

// osPipe / swapStdout are tiny indirections so the test can intercept os.Stdout.
// Kept in a separate file so lifecycle.go has no test-only imports.

func osPipe() (*os.File, *os.File, error) {
	return os.Pipe()
}

func swapStdout(w *os.File) *os.File {
	old := os.Stdout
	os.Stdout = w
	return old
}

func restoreStdout(f *os.File) {
	os.Stdout = f
}

// silence unused import warning when this file is built in non-test builds
var _ io.Reader = (*os.File)(nil)
