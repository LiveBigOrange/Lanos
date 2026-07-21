package identity

// runtimeGOOS is a tiny indirection so tests on any host can simulate
// cross-platform dataDir() without GOOS-specific builds.
// In production builds it resolves to the real GOOS at compile time.
func runtimeGOOS() string {
	return runtimeGOOSImpl()
}
