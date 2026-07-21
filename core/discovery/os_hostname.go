package discovery

import (
	"os"
	"runtime"
)

// osHostname is a small wrapper around os.Hostname() so tests can mock it
// via a build-tag-free variable swap if needed.
var osHostname = os.Hostname

// platformString returns the runtime.GOOS value (linux/darwin/windows/etc).
// Wrapped so tests can override the platform reported in Self().
var platformString = func() string { return runtime.GOOS }
