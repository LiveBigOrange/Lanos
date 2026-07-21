// Package transfer: per-device transfer queue with concurrency limits.
//
// Each peer device gets an independent FIFO queue. Within a device, at most
// maxUp uploads and maxDown downloads run concurrently (PRD §4.2.5 / roadmap
// P1-16: 4 up / 4 down). When a slot frees, the next pending task for that
// device and direction is dispatched. This bounds resource use per peer while
// keeping devices independent (one slow peer never blocks another).
package transfer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// Direction is the transfer direction relative to the local device.
type Direction int

const (
	DirectionUpload Direction = iota // local -> peer (we are the sender)
	DirectionDownload               // peer -> local (we are the receiver)
)

// String returns a human-readable direction name.
func (d Direction) String() string {
	if d == DirectionUpload {
		return "upload"
	}
	return "download"
}

// DefaultMaxConcurrentUp and DefaultMaxConcurrentDown are the per-device
// concurrency caps (roadmap P1-16: 4 up / 4 down).
const (
	DefaultMaxConcurrentUp   = 4
	DefaultMaxConcurrentDown = 4
)

// Queue errors.
var (
	ErrQueueClosed = errors.New("transfer: queue closed")
	ErrQueueEmpty  = errors.New("transfer: queue empty")
)

// TaskFunc is the work a queued transfer performs. It receives a context that
// is cancelled when the queue is closed or the task is cancelled.
type TaskFunc func(ctx context.Context) error

// Task is a queued transfer operation.
type Task struct {
	ID        string   // transfer ID (matches Manager.Transfer.ID)
	PeerID    string   // target device ID
	Direction Direction
	Execute   TaskFunc
}

// deviceQueue is the per-peer FIFO state.
type deviceQueue struct {
	pending    []*Task
	activeUp   int
	activeDown int
}

func (dq *deviceQueue) activeFor(d Direction) int {
	if d == DirectionUpload {
		return dq.activeUp
	}
	return dq.activeDown
}

func (dq *deviceQueue) incActive(d Direction) {
	if d == DirectionUpload {
		dq.activeUp++
	} else {
		dq.activeDown++
	}
}

func (dq *deviceQueue) decActive(d Direction) {
	if d == DirectionUpload {
		dq.activeUp--
	} else {
		dq.activeDown--
	}
}

// Queue schedules per-device transfer tasks with per-direction concurrency caps.
type Queue struct {
	maxUp   int
	maxDown int

	mu      sync.Mutex
	cond    *sync.Cond
	devices map[string]*deviceQueue
	closed  bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup // tracks in-flight tasks for Wait()
}

// NewQueue creates a queue with the given per-device concurrency caps. Zero or
// negative values fall back to the defaults (4/4).
func NewQueue(maxUp, maxDown int) *Queue {
	if maxUp <= 0 {
		maxUp = DefaultMaxConcurrentUp
	}
	if maxDown <= 0 {
		maxDown = DefaultMaxConcurrentDown
	}
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		maxUp:   maxUp,
		maxDown: maxDown,
		devices: make(map[string]*deviceQueue),
		ctx:     ctx,
		cancel:  cancel,
	}
	q.cond = sync.NewCond(&q.mu)
	go q.dispatchLoop()
	return q
}

// MaxUp returns the per-device upload concurrency cap.
func (q *Queue) MaxUp() int { return q.maxUp }

// MaxDown returns the per-device download concurrency cap.
func (q *Queue) MaxDown() int { return q.maxDown }

// Enqueue adds a task to its peer's queue. Tasks run as soon as a direction
// slot is available. Returns ErrQueueClosed if the queue has been closed.
func (q *Queue) Enqueue(t *Task) error {
	if t == nil || t.Execute == nil {
		return fmt.Errorf("transfer: nil task or execute func")
	}
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return ErrQueueClosed
	}
	dq, ok := q.devices[t.PeerID]
	if !ok {
		dq = &deviceQueue{}
		q.devices[t.PeerID] = dq
	}
	dq.pending = append(dq.pending, t)
	q.mu.Unlock()
	q.cond.Signal()
	return nil
}

// dispatchLoop wakes whenever a task is enqueued or completes, and starts any
// runnable tasks (pending task whose direction has a free slot). It exits when
// the queue is closed.
func (q *Queue) dispatchLoop() {
	for {
		q.mu.Lock()
		for !q.closed && !q.hasRunnableLocked() {
			q.cond.Wait()
		}
		if q.closed {
			q.mu.Unlock()
			return
		}
		t := q.popRunnableLocked()
		q.mu.Unlock()
		if t != nil {
			q.runTask(t)
		}
	}
}

// hasRunnableLocked reports whether any device has a pending task with a free
// direction slot. Caller must hold q.mu.
func (q *Queue) hasRunnableLocked() bool {
	for _, dq := range q.devices {
		if q.runnableInLocked(dq) != nil {
			return true
		}
	}
	return false
}

// runnableInLocked returns the first runnable task in dq, or nil. Caller holds q.mu.
func (q *Queue) runnableInLocked(dq *deviceQueue) *Task {
	for _, t := range dq.pending {
		cap := q.maxUp
		if t.Direction == DirectionDownload {
			cap = q.maxDown
		}
		if dq.activeFor(t.Direction) < cap {
			return t
		}
	}
	return nil
}

// popRunnableLocked finds and removes the first runnable task across all
// devices (round-robin over device map), increments its active counter, and
// returns it. Caller must hold q.mu.
func (q *Queue) popRunnableLocked() *Task {
	for _, dq := range q.devices {
		for i, t := range dq.pending {
			cap := q.maxUp
			if t.Direction == DirectionDownload {
				cap = q.maxDown
			}
			if dq.activeFor(t.Direction) < cap {
				dq.pending = append(dq.pending[:i], dq.pending[i+1:]...)
				dq.incActive(t.Direction)
				return t
			}
		}
	}
	return nil
}

// runTask executes a task in a goroutine, freeing its slot on completion.
func (q *Queue) runTask(t *Task) {
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		defer q.releaseSlot(t)
		if err := t.Execute(q.ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("transfer task failed", "id", t.ID, "peer", t.PeerID, "dir", t.Direction, "err", err)
		}
	}()
}

// releaseSlot decrements the task's device/direction counter and signals the
// dispatcher to start the next pending task.
func (q *Queue) releaseSlot(t *Task) {
	q.mu.Lock()
	if dq, ok := q.devices[t.PeerID]; ok {
		dq.decActive(t.Direction)
		// Drop the device entry once fully drained to avoid unbounded growth.
		if len(dq.pending) == 0 && dq.activeUp == 0 && dq.activeDown == 0 {
			delete(q.devices, t.PeerID)
		}
	}
	q.mu.Unlock()
	q.cond.Signal()
}

// Pending returns the count of tasks waiting (not yet running) for a device.
func (q *Queue) Pending(peerID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	dq, ok := q.devices[peerID]
	if !ok {
		return 0
	}
	return len(dq.pending)
}

// Active returns the number of running tasks (up+down) for a device.
func (q *Queue) Active(peerID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	dq, ok := q.devices[peerID]
	if !ok {
		return 0
	}
	return dq.activeUp + dq.activeDown
}

// DeviceCount returns the number of devices with pending or active tasks.
func (q *Queue) DeviceCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.devices)
}

// Wait blocks until all enqueued tasks have completed. Call after Close to
// drain in-flight work.
func (q *Queue) Wait() {
	q.wg.Wait()
}

// Close stops accepting new tasks and cancels in-flight ones. Pending tasks
// are abandoned. Use Wait to drain in-flight tasks.
func (q *Queue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	q.mu.Unlock()
	q.cond.Broadcast()
	q.cancel()
}
