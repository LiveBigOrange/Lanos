package transfer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// makeTask builds a Task that, when run, increments active, blocks until the
// release channel is closed, records its start order, then decrements active.
func makeTask(id, peer string, dir Direction, active *int32, order *[]string, mu *sync.Mutex, release <-chan struct{}) *Task {
	return &Task{
		ID:        id,
		PeerID:    peer,
		Direction: dir,
		Execute: func(ctx context.Context) error {
			atomic.AddInt32(active, 1)
			mu.Lock()
			*order = append(*order, id)
			mu.Unlock()
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			atomic.AddInt32(active, -1)
			return nil
		},
	}
}

func TestQueueRespectsConcurrencyCap(t *testing.T) {
	q := NewQueue(4, 4)
	var active int32
	var maxActive int32
	var doneCount int32
	release := make(chan struct{})

	const n = 10
	for i := 0; i < n; i++ {
		tk := &Task{
			ID: taskName(i), PeerID: "peer-A", Direction: DirectionUpload,
			Execute: func(ctx context.Context) error {
				cur := atomic.AddInt32(&active, 1)
				for {
					m := atomic.LoadInt32(&maxActive)
					if cur <= m || atomic.CompareAndSwapInt32(&maxActive, m, cur) {
						break
					}
				}
				<-release
				atomic.AddInt32(&active, -1)
				atomic.AddInt32(&doneCount, 1)
				return nil
			},
		}
		if err := q.Enqueue(tk); err != nil {
			t.Fatal(err)
		}
	}

	// With cap 4, exactly 4 should be running, 6 pending.
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&active); got != 4 {
		t.Fatalf("expected 4 active, got %d", got)
	}
	if got := q.Pending("peer-A"); got != 6 {
		t.Fatalf("expected 6 pending, got %d", got)
	}

	close(release)
	q.Wait()

	if got := atomic.LoadInt32(&maxActive); got > 4 {
		t.Fatalf("concurrency exceeded cap: max observed %d > 4", got)
	}
	if got := atomic.LoadInt32(&doneCount); got != n {
		t.Fatalf("expected %d completions, got %d", got, n)
	}
	if got := atomic.LoadInt32(&active); got != 0 {
		t.Fatalf("expected 0 active after drain, got %d", got)
	}
}

// TestQueueFIFOOrderWithCap1 verifies strict FIFO ordering when only one task
// runs at a time (cap=1 makes goroutine start order deterministic).
func TestQueueFIFOOrderWithCap1(t *testing.T) {
	q := NewQueue(1, 1)
	var mu sync.Mutex
	var order []string
	for i := 0; i < 5; i++ {
		name := taskName(i)
		q.Enqueue(&Task{
			ID: name, PeerID: "peer", Direction: DirectionUpload,
			Execute: func(context.Context) error {
				mu.Lock()
				order = append(order, name)
				mu.Unlock()
				return nil
			},
		})
	}
	q.Wait()
	want := []string{"a", "b", "c", "d", "e"}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != len(want) {
		t.Fatalf("got %d completions, want %d", len(order), len(want))
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("FIFO order wrong: got %v want %v", order, want)
		}
	}
}

func TestQueueDirectionIndependence(t *testing.T) {
	// One device, 4 up + 4 down = 8 concurrent allowed.
	q := NewQueue(4, 4)
	var active int32
	var mu sync.Mutex
	var order []string
	release := make(chan struct{})

	for i := 0; i < 4; i++ {
		q.Enqueue(makeTask("up-"+taskName(i), "peer", DirectionUpload, &active, &order, &mu, release))
		q.Enqueue(makeTask("dn-"+taskName(i), "peer", DirectionDownload, &active, &order, &mu, release))
	}

	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&active); got != 8 {
		t.Fatalf("expected 8 active (4 up + 4 down), got %d", got)
	}
	close(release)
	q.Wait()
	if len(order) != 8 {
		t.Fatalf("expected 8 completions, got %d", len(order))
	}
}

func TestQueuePerDeviceIndependence(t *testing.T) {
	// Two devices, each cap 4 up. 4 + 4 = 8 should run at once.
	q := NewQueue(4, 4)
	var active int32
	var mu sync.Mutex
	var order []string
	release := make(chan struct{})

	for d := 0; d < 2; d++ {
		peer := "peer-" + taskName(d)
		for i := 0; i < 4; i++ {
			q.Enqueue(makeTask(peer+"-"+taskName(i), peer, DirectionUpload, &active, &order, &mu, release))
		}
	}
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&active); got != 8 {
		t.Fatalf("expected 8 active across 2 devices, got %d", got)
	}
	close(release)
	q.Wait()
	if len(order) != 8 {
		t.Fatalf("expected 8 completions, got %d", len(order))
	}
}

func TestQueueCloseCancelsInFlight(t *testing.T) {
	q := NewQueue(2, 2)
	var active int32
	var mu sync.Mutex
	var order []string
	release := make(chan struct{}) // never closed
	done := make(chan struct{})

	q.Enqueue(makeTask("t1", "peer", DirectionUpload, &active, &order, &mu, release))
	q.Enqueue(makeTask("t2", "peer", DirectionUpload, &active, &order, &mu, release))

	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&active); got != 2 {
		t.Fatalf("expected 2 active, got %d", got)
	}

	// Close cancels the task contexts; they should exit via ctx.Done().
	go func() {
		q.Close()
		q.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel in-flight tasks")
	}
}

func TestQueueEnqueueAfterClose(t *testing.T) {
	q := NewQueue(2, 2)
	q.Close()
	err := q.Enqueue(&Task{ID: "x", PeerID: "p", Direction: DirectionUpload, Execute: func(context.Context) error { return nil }})
	if !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("got %v, want ErrQueueClosed", err)
	}
}

func TestQueueRejectsNilTask(t *testing.T) {
	q := NewQueue(2, 2)
	defer q.Close()
	if err := q.Enqueue(nil); err == nil {
		t.Fatal("expected error for nil task")
	}
	if err := q.Enqueue(&Task{ID: "x", PeerID: "p", Direction: DirectionUpload}); err == nil {
		t.Fatal("expected error for nil Execute")
	}
}

func TestQueueDrainsEmptyDevice(t *testing.T) {
	// After tasks complete, the device entry should be removed.
	q := NewQueue(2, 2)
	done := make(chan struct{})
	q.Enqueue(&Task{
		ID: "t", PeerID: "p", Direction: DirectionUpload,
		Execute: func(context.Context) error { <-done; return nil },
	})
	time.Sleep(50 * time.Millisecond)
	if q.DeviceCount() != 1 {
		t.Fatalf("expected 1 device, got %d", q.DeviceCount())
	}
	close(done)
	q.Wait()
	if q.DeviceCount() != 0 {
		t.Fatalf("expected 0 devices after drain, got %d", q.DeviceCount())
	}
	if q.Active("p") != 0 || q.Pending("p") != 0 {
		t.Fatal("device state not zero after drain")
	}
}

func TestDirectionString(t *testing.T) {
	if DirectionUpload.String() != "upload" {
		t.Fatal("upload string")
	}
	if DirectionDownload.String() != "download" {
		t.Fatal("download string")
	}
}

func taskName(i int) string {
	return string(rune('a' + i))
}
