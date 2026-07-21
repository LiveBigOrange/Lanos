package integration

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"
)

// TestTransfer_IPv6PerfBaseline runs a multi-MB encrypted transfer over
// [::1] loopback and logs the measured throughput as a baseline. It does
// not assert a throughput floor (CI runner variance makes that flaky) but
// fails on data corruption or transfer stall.
//
// Use `go test -run IPv6PerfBaseline -v ./integration` to record a number
// for regression tracking. If the elapsed time grows multiples between
// releases, investigate per docs/NETWORK.md §1.
func TestTransfer_IPv6PerfBaseline(t *testing.T) {
	t.Parallel()

	const size = 8 * 1024 * 1024 // 8 MiB — large enough to amortize handshake cost.

	_, data, hash := randFile(t, size)

	sender, receiver := establishPairOn(t, "[::1]:0")
	defer sender.Close()
	defer receiver.Close()

	start := time.Now()

	doneSend := make(chan struct{})
	go func() {
		defer close(doneSend)
		sendFile(t, sender, "perf-v6", "perf_v6.bin", data)
	}()

	_, body := recvFile(t, receiver, int64(size))
	<-doneSend

	elapsed := time.Since(start)
	mbps := float64(size) / 1024 / 1024 / elapsed.Seconds()

	if !bytes.Equal(body, data) {
		t.Fatalf("body mismatch: got len=%d want=%d", len(body), len(data))
	}
	if got := sha256.Sum256(body); got != hash {
		t.Fatalf("sha256 mismatch: got %x want %x", got, hash)
	}

	t.Logf("IPv6 [::1] transfer baseline: %d bytes in %v (%.1f MiB/s)",
		size, elapsed.Round(time.Millisecond), mbps)
}
