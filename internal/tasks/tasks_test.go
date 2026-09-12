package tasks

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueProcessesTask(t *testing.T) {
	var calls int32
	q := New("extract_event", func(ctx context.Context, targetID string) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	q.Start()
	defer q.Stop()

	task := q.Enqueue("event-1")
	waitForStatus(t, q, "event-1", StatusSucceeded, 2*time.Second)
	if task.Status != StatusQueued && task.Status != StatusProcessing && task.Status != StatusSucceeded {
		t.Fatalf("unexpected enqueue state %q", task.Status)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("worker ran %d times, want 1", got)
	}
}

func TestQueueRetriesThenSucceeds(t *testing.T) {
	var calls int32
	q := New("extract_event", func(ctx context.Context, targetID string) error {
		if atomic.AddInt32(&calls, 1) < 3 {
			return errors.New("model timeout")
		}
		return nil
	})
	q.Start()
	defer q.Stop()

	q.Enqueue("event-1")
	waitForStatus(t, q, "event-1", StatusSucceeded, 15*time.Second)

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("worker ran %d times, want 3", got)
	}
	task := q.Get("event-1")
	if task.Attempts != 3 {
		t.Fatalf("attempts = %d, want 3", task.Attempts)
	}
	if task.LastError != "" {
		t.Fatalf("succeeded task should not keep last_error, got %q", task.LastError)
	}
}

func TestQueueFailsAfterMaxAttempts(t *testing.T) {
	var calls int32
	q := New("extract_event", func(ctx context.Context, targetID string) error {
		atomic.AddInt32(&calls, 1)
		return errors.New("model down")
	})
	q.Start()
	defer q.Stop()

	q.Enqueue("event-1")
	deadline := time.Now().Add(20 * time.Second)
	for {
		task := q.Get("event-1")
		if task != nil && task.Status == StatusFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task never failed, last state %+v", q.Get("event-1"))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&calls); got != MaxAttempts {
		t.Fatalf("worker ran %d times, want %d", got, MaxAttempts)
	}
}

func TestQueueDedupesActiveTarget(t *testing.T) {
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(1)
	q := New("extract_event", func(ctx context.Context, targetID string) error {
		started.Done()
		<-release
		return nil
	})
	q.Start()
	defer q.Stop()

	first := q.Enqueue("event-1")
	second := q.Enqueue("event-1")
	if first.ID != second.ID {
		t.Fatalf("active target was enqueued twice: %s vs %s", first.ID, second.ID)
	}
	// Wait until the worker is inside fn, then let it finish.
	started.Wait()
	close(release)
	waitForStatus(t, q, "event-1", StatusSucceeded, 2*time.Second)
}

func TestQueueRequeuesAfterFailure(t *testing.T) {
	var calls int32
	q := New("extract_event", func(ctx context.Context, targetID string) error {
		atomic.AddInt32(&calls, 1)
		return errors.New("always fails")
	})
	q.Start()
	defer q.Stop()

	q.Enqueue("event-1")
	waitForStatus(t, q, "event-1", StatusFailed, 20*time.Second)

	// After the failure the guard is released, so a manual retry can run again.
	again := q.Enqueue("event-1")
	_ = again
	waitForStatus(t, q, "event-1", StatusFailed, 20*time.Second)
	if got := atomic.LoadInt32(&calls); got != 2*MaxAttempts {
		t.Fatalf("worker ran %d times, want %d", got, 2*MaxAttempts)
	}
}

func TestSnapshotReturnsNewestFirst(t *testing.T) {
	q := New("extract_event", func(ctx context.Context, targetID string) error { return nil })
	q.Start()
	defer q.Stop()

	q.Enqueue("event-1")
	q.Enqueue("event-2")
	waitForStatus(t, q, "event-2", StatusSucceeded, 2*time.Second)
	waitForStatus(t, q, "event-1", StatusSucceeded, 2*time.Second)

	snap := q.Snapshot(10)
	if len(snap) != 2 {
		t.Fatalf("snapshot has %d tasks, want 2", len(snap))
	}
	if snap[0].CreatedAt < snap[1].CreatedAt {
		t.Fatalf("snapshot not newest first: %s before %s", snap[0].CreatedAt, snap[1].CreatedAt)
	}
}

// TestStatsCounters verifies the observability contract: lifetime counters,
// latency fields and current depth add up to what actually happened.
func TestStatsCounters(t *testing.T) {
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(1)
	q := New("extract_event", func(ctx context.Context, targetID string) error {
		if targetID == "event-slow" {
			started.Done()
			<-release
		}
		return nil
	})
	q.Start()
	defer q.Stop()

	q.Enqueue("event-slow")
	started.Wait() // worker inside fn: one task processing
	s := q.Stats()
	if s.Enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1", s.Enqueued)
	}
	if s.ProcessingNow != 1 || s.QueuedNow != 0 {
		t.Fatalf("depth = processing %d / queued %d, want 1/0", s.ProcessingNow, s.QueuedNow)
	}

	// The single worker is still blocked inside event-slow, so the next
	// enqueue must surface as queued depth, not processing.
	q.Enqueue("event-fast")
	s = q.Stats()
	if s.Enqueued != 2 || s.ProcessingNow != 1 || s.QueuedNow != 1 {
		t.Fatalf("depth = enqueued %d / processing %d / queued %d, want 2/1/1",
			s.Enqueued, s.ProcessingNow, s.QueuedNow)
	}

	// Unblock the slow task so both tasks finish and the worker drains.
	close(release)
	waitForStatus(t, q, "event-fast", StatusSucceeded, 2*time.Second)
	waitForStatus(t, q, "event-slow", StatusSucceeded, 2*time.Second)

	s = q.Stats()
	if s.Enqueued != 2 || s.Succeeded != 2 || s.Failed != 0 {
		t.Fatalf("counters = enqueued %d / succeeded %d / failed %d, want 2/2/0",
			s.Enqueued, s.Succeeded, s.Failed)
	}
	if s.LastDurationMS < 0 || s.AvgDurationMS < 0 {
		t.Fatalf("durations must be non-negative, got last %d avg %d", s.LastDurationMS, s.AvgDurationMS)
	}
	if s.ProcessingNow != 0 || s.QueuedNow != 0 {
		t.Fatalf("queue should be idle, got processing %d queued %d", s.ProcessingNow, s.QueuedNow)
	}
}

// waitForStatus polls until the latest task for target reaches want. The queue
// has no notification channel, so tests poll; the worker runs in a goroutine.
func waitForStatus(t *testing.T, q *Queue, target, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		task := q.Get(target)
		if task != nil && task.Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("task for %s never reached %s, state: %+v", target, want, task)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
