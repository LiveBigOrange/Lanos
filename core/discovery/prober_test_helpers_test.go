package discovery

import (
	"io"
	"log/slog"
	"net"
)

// testLogger returns a discard logger for quiet tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// netSplitHostPort wraps net.SplitHostPort returning the port as int.
func netSplitHostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
