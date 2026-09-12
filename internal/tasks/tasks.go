// Package tasks provides a small in-process task queue for work that must not
// block an HTTP request — currently the AI extraction pipeline. It is single
// worker by design: the LLM calls behind it are slow and the SQLite store is
// single-connection, so extra parallelism would buy nothing but lock contention.
//
// Durability model: queue state is in memory only. The source of truth is the
// database row the task targets (an event stuck in extraction_status='pending'),
// so a crash or shutdown loses at most ordering, never work — startup recovery
// re-enqueues every pending row.
package tasks

import (
	"context"
	"log"
	"sync"
	"time"
)

// Task states. queued/processing live only in memory; succeeded/failed describe
// the last completed attempt for a target.
const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
)

// MaxAttempts bounds automatic retries per task. Attempt 1 runs immediately,
// later attempts back off; the retryable failures here are transient model or
// network errors, and a broken configuration keeps failing no matter how often.
const MaxAttempts = 3

// Task is one unit of work. TargetID is the event whose extraction the task
// runs; the same target can have at most one active (queued/processing) task.
type Task struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	TargetID    string  `json:"target_id"`
	Status      string  `json:"status"`
	Attempts    int     `json:"attempts"`
	LastError   string  `json:"last_error,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// Stats is the queue-level health view: lifetime counters plus how long
// completed work took. Failure rate and latency become numbers instead of
// impressions — a queue that silently stopped is visible here.
type Stats struct {
	Enqueued       int   `json:"enqueued"`
	Succeeded      int   `json:"succeeded"`
	Failed         int   `json:"failed"`
	QueuedNow      int   `json:"queued_now"`
	ProcessingNow  int   `json:"processing_now"`
	LastDurationMS int64 `json:"last_duration_ms"`
	AvgDurationMS  int64 `json:"avg_duration_ms"`
}

// Queue is the in-process executor. Create with New, then Start, then Enqueue.
type Queue struct {
	taskType string
	fn       func(ctx context.Context, targetID string) error

	mu      sync.Mutex
	order   []string          // task ids, enqueue order (snapshot stability)
	tasks   map[string]*Task  // task id -> task
	active  map[string]string // target id -> active task id
	ch      chan string       // wakes the worker
	wg      sync.WaitGroup
	baseCtx context.Context
	cancel  context.CancelFunc
	stopped bool

	// Lifetime counters (guarded by mu).
	enqueued  int
	succeeded int
	failed    int
	lastDur   time.Duration
	totalDur  time.Duration
	doneRuns  int
}

// buffer 1024: personal-scale use never gets close; a full buffer would mean
// thousands of unprocessed records, which is a recovery problem, not a queue
// sizing problem.
const queueBuffer = 1024

// New builds a queue that runs fn(targetID) for every enqueued task. fn must be
// idempotent: retries and crash recovery can run it more than once per target.
func New(taskType string, fn func(ctx context.Context, targetID string) error) *Queue {
	ctx, cancel := context.WithCancel(context.Background())
	return &Queue{
		taskType: taskType,
		fn:       fn,
		tasks:    make(map[string]*Task),
		active:   make(map[string]string),
		ch:       make(chan string, queueBuffer),
		baseCtx:  ctx,
		cancel:   cancel,
	}
}

// Start launches the single worker goroutine.
func (q *Queue) Start() {
	q.wg.Add(1)
	go q.worker()
}

// Enqueue schedules fn(targetID). If the target already has a queued or
// processing task, the existing one is returned and nothing new is scheduled —
// this is the idempotency guard against double-submits and startup recovery
// racing a live enqueue.
func (q *Queue) Enqueue(targetID string) *Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	if id, ok := q.active[targetID]; ok {
		return q.tasks[id]
	}
	if q.stopped {
		t := q.newTaskLocked(targetID)
		t.Status = StatusFailed
		t.LastError = "queue stopped"
		now := nowUTC()
		t.CompletedAt = &now
		q.rememberLocked(t)
		q.failed++
		return t
	}
	t := q.newTaskLocked(targetID)
	t.Status = StatusQueued
	q.rememberLocked(t)
	q.active[targetID] = t.ID
	q.enqueued++
	q.ch <- t.ID
	return t
}

func (q *Queue) newTaskLocked(targetID string) *Task {
	now := nowUTC()
	return &Task{
		ID:        targetID + ":" + now, // readable and unique enough per target restart
		Type:      q.taskType,
		TargetID:  targetID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (q *Queue) rememberLocked(t *Task) {
	q.tasks[t.ID] = t
	q.order = append(q.order, t.ID)
	if len(q.order) > 500 {
		// Snapshot history is diagnostic only; the oldest entries can go.
		for _, id := range q.order[:100] {
			delete(q.tasks, id)
		}
		q.order = append([]string{}, q.order[100:]...)
	}
}

// Get returns the latest task for a target, if any.
func (q *Queue) Get(targetID string) *Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	if id, ok := q.active[targetID]; ok {
		t := q.tasks[id]
		cp := *t
		return &cp
	}
	// Fall back to the most recent finished task for this target.
	for i := len(q.order) - 1; i >= 0; i-- {
		t := q.tasks[q.order[i]]
		if t.TargetID == targetID {
			cp := *t
			return &cp
		}
	}
	return nil
}

// Snapshot returns up to limit recent tasks, newest first.
func (q *Queue) Snapshot(limit int) []*Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	out := make([]*Task, 0, limit)
	for i := len(q.order) - 1; i >= 0 && len(out) < limit; i-- {
		t := q.tasks[q.order[i]]
		cp := *t
		out = append(out, &cp)
	}
	return out
}

// Stop cancels the in-flight task, stops accepting new ones and waits for the
// worker to exit. Unprocessed queued tasks are simply dropped: their targets
// stay in the state that recovery re-enqueues from.
func (q *Queue) Stop() {
	q.mu.Lock()
	if q.stopped {
		q.mu.Unlock()
		return
	}
	q.stopped = true
	q.mu.Unlock()

	q.cancel()
	close(q.ch)
	q.wg.Wait()
}

func (q *Queue) worker() {
	defer q.wg.Done()
	for id := range q.ch {
		if ctx := q.baseCtx; ctx.Err() != nil {
			// Shutdown during backlog: mark the rest failed so nothing looks stuck.
			q.finish(id, StatusFailed, "cancelled: shutdown")
			continue
		}
		q.run(id)
	}
}

// run executes one task with bounded retries. The event row keeps
// extraction_status='pending' while attempts run, so a crash mid-run is
// recovered by startup re-enqueue, never half-applied: RetryExtraction either
// stores a complete extraction or records a failure.
func (q *Queue) run(id string) {
	q.mu.Lock()
	t, ok := q.tasks[id]
	if !ok {
		q.mu.Unlock()
		return
	}
	target := t.TargetID
	t.Status = StatusProcessing
	t.Attempts++
	t.UpdatedAt = nowUTC()
	q.mu.Unlock()
	runStart := time.Now()
	defer func() { q.recordRun(time.Since(runStart)) }()

	backoff := 2 * time.Second
	var lastErr error
	for attempt := t.Attempts; attempt <= MaxAttempts; attempt++ {
		q.mu.Lock()
		t.Attempts = attempt
		t.UpdatedAt = nowUTC()
		q.mu.Unlock()

		if err := q.fn(q.baseCtx, target); err != nil {
			lastErr = err
			if q.baseCtx.Err() != nil {
				// Shutdown: leave the target pending; recovery will re-enqueue.
				q.mu.Lock()
				t.LastError = "cancelled: shutdown"
				t.UpdatedAt = nowUTC()
				q.mu.Unlock()
				q.release(target, id)
				return
			}
			log.Printf("warning: task %s attempt %d/%d failed: %v", id, attempt, MaxAttempts, err)
			if attempt < MaxAttempts {
				q.mu.Lock()
				t.LastError = err.Error()
				q.mu.Unlock()
				select {
				case <-time.After(backoff):
				case <-q.baseCtx.Done():
				}
				backoff *= 3
				continue
			}
		} else {
			q.finish(id, StatusSucceeded, "")
			q.release(target, id)
			return
		}
	}
	q.finish(id, StatusFailed, lastErr.Error())
	q.release(target, id)
}

func (q *Queue) finish(id, status, errMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.tasks[id]
	if !ok {
		return
	}
	t.Status = status
	t.LastError = errMsg
	now := nowUTC()
	t.UpdatedAt = now
	t.CompletedAt = &now
	switch status {
	case StatusSucceeded:
		q.succeeded++
	case StatusFailed:
		q.failed++
	}
}

// recordRun adds one completed run's wall time to the latency view. Shutdown
// cancellations are counted too: from the outside they are time the work took
// without finishing.
func (q *Queue) recordRun(d time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.lastDur = d
	q.totalDur += d
	q.doneRuns++
}

// Stats snapshots the counters. Safe to call at any time, including before
// Start and after Stop.
func (q *Queue) Stats() Stats {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := Stats{
		Enqueued:  q.enqueued,
		Succeeded: q.succeeded,
		Failed:    q.failed,
	}
	for _, id := range q.active {
		switch t := q.tasks[id]; {
		case t == nil:
		case t.Status == StatusProcessing:
			s.ProcessingNow++
		case t.Status == StatusQueued:
			s.QueuedNow++
		}
	}
	if q.doneRuns > 0 {
		s.LastDurationMS = q.lastDur.Milliseconds()
		s.AvgDurationMS = (q.totalDur / time.Duration(q.doneRuns)).Milliseconds()
	}
	return s
}

// release detaches the active-task guard so the same target can be enqueued
// again (e.g. a manual retry after a failure).
func (q *Queue) release(target, id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active[target] == id {
		delete(q.active, target)
	}
}

func nowUTC() string {
	return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
}
