// Package transfer: per-transfer cancellation and temp-data cleanup.
//
// Each in-flight transfer registers its context.CancelFunc and an optional
// cleanup callback (typically ChunkWriter.Cleanup, which removes the
// transfer_cache/<task_id>/ chunk cache). Any party may cancel a transfer by
// ID; cancellation triggers both the context cancellation (so the transfer
// goroutine unblocks) and the cleanup (so partial chunk data is purged). This
// satisfies roadmap P1-18: "任一方取消后 chunk cache 清空".
package transfer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// ErrCancelNotRegistered is returned when a cancel/cleanup is requested for an
// unknown transfer ID.
var ErrCancelNotRegistered = errors.New("transfer: transfer not registered for cancel")

// CancelRegistry tracks in-flight transfers so any one can be cancelled by ID,
// triggering its context cancellation and registered cleanup.
type CancelRegistry struct {
	mu      sync.Mutex
	entries map[string]*cancelEntry
}

type cancelEntry struct {
	cancel     context.CancelFunc
	cleanup    func()
	cleanupRan bool
}

// NewCancelRegistry creates an empty registry.
func NewCancelRegistry() *CancelRegistry {
	return &CancelRegistry{entries: make(map[string]*cancelEntry)}
}

// Register associates a transfer ID with its cancel func and cleanup callback.
// The cleanup callback is invoked exactly once, either on Cancel (failure) or
// explicitly via Cleanup. On a successful transfer the caller MUST call
// Complete to remove the entry without running cleanup (the assembled file is
// kept; only the chunk cache is removed via a separate Cleanup call if desired).
//
// Registering the same ID twice replaces the previous entry. The previous
// entry's cleanup (if not yet run) is invoked BEFORE the new entry is stored,
// to avoid leaking the previous transfer's temp chunk data. The previous
// entry's cancel func is NOT invoked — the caller of Register is presumed to
// have obtained the new cancel func from a fresh context and is responsible
// for keeping the new lifecycle consistent.
func (r *CancelRegistry) Register(id string, cancel context.CancelFunc, cleanup func()) {
	r.mu.Lock()
	old := r.entries[id]
	r.entries[id] = &cancelEntry{cancel: cancel, cleanup: cleanup}
	r.mu.Unlock()
	if old != nil {
		old.runCleanup()
	}
}

// Cancel cancels the transfer's context and runs its cleanup callback (purging
// temp chunk data). Idempotent for the cleanup: calling Cancel twice runs the
// cleanup only once. Returns ErrCancelNotRegistered if id is unknown.
func (r *CancelRegistry) Cancel(id string) error {
	r.mu.Lock()
	e, ok := r.entries[id]
	if !ok {
		r.mu.Unlock()
		return ErrCancelNotRegistered
	}
	delete(r.entries, id)
	r.mu.Unlock()

	if e.cancel != nil {
		e.cancel()
	}
	e.runCleanup()
	slog.Info("transfer cancelled + cleanup", "id", id)
	return nil
}

// Cleanup runs only the cleanup callback (e.g. after a failed transfer that was
// not user-cancelled) without cancelling the context. Returns
// ErrCancelNotRegistered if id is unknown.
func (r *CancelRegistry) Cleanup(id string) error {
	r.mu.Lock()
	e, ok := r.entries[id]
	if !ok {
		r.mu.Unlock()
		return ErrCancelNotRegistered
	}
	delete(r.entries, id)
	r.mu.Unlock()

	e.runCleanup()
	return nil
}

// Complete marks a transfer as finished successfully: it removes the entry
// without cancelling or cleaning up. The caller may invoke Cleanup separately
// if it wants to purge the chunk cache after a successful assembly.
func (r *CancelRegistry) Complete(id string) {
	r.mu.Lock()
	delete(r.entries, id)
	r.mu.Unlock()
}

// IsRegistered reports whether id currently has an active entry.
func (r *CancelRegistry) IsRegistered(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[id]
	return ok
}

// ActiveCount returns the number of registered (in-flight) transfers.
func (r *CancelRegistry) ActiveCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// CancelAll cancels every registered transfer and runs their cleanups. Used on
// shutdown.
func (r *CancelRegistry) CancelAll() {
	r.mu.Lock()
	entries := r.entries
	r.entries = make(map[string]*cancelEntry)
	r.mu.Unlock()

	for id, e := range entries {
		if e.cancel != nil {
			e.cancel()
		}
		e.runCleanup()
		slog.Info("transfer cancelled (cancel-all) + cleanup", "id", id)
	}
}

// runCleanup runs the cleanup callback exactly once, recovering from panics.
func (e *cancelEntry) runCleanup() {
	if e.cleanupRan {
		return
	}
	e.cleanupRan = true
	if e.cleanup != nil {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("transfer cleanup panic", "recover", fmt.Sprint(rec))
			}
		}()
		e.cleanup()
	}
}
