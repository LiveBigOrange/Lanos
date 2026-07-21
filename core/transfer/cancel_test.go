package transfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCancelRegistryCancelRunsCleanup(t *testing.T) {
	reg := NewCancelRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	cleanupRan := false

	// Create a fake chunk cache dir to prove cleanup purges it.
	cacheDir := t.TempDir()
	chunkDir := filepath.Join(cacheDir, "task-1")
	if err := os.MkdirAll(chunkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chunkDir, "chunk_0"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg.Register("task-1", cancel, func() {
		cleanupRan = true
		os.RemoveAll(chunkDir)
	})

	if !reg.IsRegistered("task-1") {
		t.Fatal("should be registered")
	}
	if err := reg.Cancel("task-1"); err != nil {
		t.Fatal(err)
	}

	if !cleanupRan {
		t.Fatal("cleanup callback did not run")
	}
	if ctx.Err() == nil {
		t.Fatal("context was not cancelled")
	}
	if _, err := os.Stat(chunkDir); !os.IsNotExist(err) {
		t.Fatalf("chunk cache dir should be removed, got err=%v", err)
	}
	if reg.IsRegistered("task-1") {
		t.Fatal("entry should be removed after cancel")
	}
}

func TestCancelRegistryCancelUnknownID(t *testing.T) {
	reg := NewCancelRegistry()
	err := reg.Cancel("nope")
	if !errors.Is(err, ErrCancelNotRegistered) {
		t.Fatalf("got %v, want ErrCancelNotRegistered", err)
	}
}

func TestCancelRegistryCleanupOnly(t *testing.T) {
	reg := NewCancelRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	cleanupRan := false
	reg.Register("t", cancel, func() { cleanupRan = true })

	if err := reg.Cleanup("t"); err != nil {
		t.Fatal(err)
	}
	if !cleanupRan {
		t.Fatal("cleanup did not run")
	}
	if ctx.Err() != nil {
		t.Fatal("context should NOT be cancelled by Cleanup-only")
	}
}

func TestCancelRegistryCompleteRemovesWithoutCleanup(t *testing.T) {
	reg := NewCancelRegistry()
	cleanupRan := false
	reg.Register("t", func() {}, func() { cleanupRan = true })

	reg.Complete("t")
	if reg.IsRegistered("t") {
		t.Fatal("entry should be removed by Complete")
	}
	if cleanupRan {
		t.Fatal("cleanup should NOT run on Complete")
	}
}

func TestCancelRegistryIdempotentCleanup(t *testing.T) {
	reg := NewCancelRegistry()
	count := 0
	reg.Register("t", func() {}, func() { count++ })

	if err := reg.Cancel("t"); err != nil {
		t.Fatal(err)
	}
	// Second cancel is "not registered" since the entry was removed.
	if err := reg.Cancel("t"); !errors.Is(err, ErrCancelNotRegistered) {
		t.Fatalf("second cancel: got %v, want ErrCancelNotRegistered", err)
	}
	if count != 1 {
		t.Fatalf("cleanup ran %d times, want 1", count)
	}
}

func TestCancelRegistryCancelAll(t *testing.T) {
	reg := NewCancelRegistry()
	var mu sync.Mutex
	ran := map[string]bool{}
	ctx1, c1 := context.WithCancel(context.Background())
	ctx2, c2 := context.WithCancel(context.Background())
	reg.Register("a", c1, func() { mu.Lock(); ran["a"] = true; mu.Unlock() })
	reg.Register("b", c2, func() { mu.Lock(); ran["b"] = true; mu.Unlock() })

	reg.CancelAll()
	if ctx1.Err() == nil || ctx2.Err() == nil {
		t.Fatal("contexts not cancelled")
	}
	mu.Lock()
	if !ran["a"] || !ran["b"] {
		t.Fatalf("not all cleanups ran: %v", ran)
	}
	mu.Unlock()
	if reg.ActiveCount() != 0 {
		t.Fatal("registry should be empty after CancelAll")
	}
}

func TestCancelRegistryRecoverPanic(t *testing.T) {
	reg := NewCancelRegistry()
	reg.Register("t", func() {}, func() { panic("boom") })
	// Should not propagate the panic.
	if err := reg.Cancel("t"); err != nil {
		t.Fatal(err)
	}
}

func TestCancelIntegrationWithChunkWriter(t *testing.T) {
	// End-to-end: a ChunkWriter's cache is purged when the transfer is cancelled.
	reg := NewCancelRegistry()
	ctx, cancel := context.WithCancel(context.Background())

	dir := t.TempDir()
	manifest := Manifest{TaskID: "task-x", FileName: "f", TotalSize: 10, ChunkCount: 1, ChunkHashes: []string{ChunkHashHex([]byte("0123456789"))}}
	cw, err := NewChunkWriter(dir, "task-x", manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := cw.WriteChunk(0, []byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	// Cache dir + meta should exist.
	cacheDir := filepath.Join(dir, "task-x")
	if _, err := os.Stat(cacheDir); err != nil {
		t.Fatal("cache dir should exist")
	}

	reg.Register("task-x", cancel, func() { cw.Cleanup() })

	// Simulate the transfer goroutine watching ctx.
	go func() {
		<-ctx.Done()
	}()

	// Cancel from "the other side".
	if err := reg.Cancel("task-x"); err != nil {
		t.Fatal(err)
	}

	// Give the watcher a moment to observe cancellation.
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("ctx not done after cancel")
	}
	// Chunk cache must be gone (DoD: 任一方取消后 chunk cache 清空).
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatalf("chunk cache should be cleared after cancel, err=%v", err)
	}
}
