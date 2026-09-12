package service

import (
	"context"
	"log"

	"relationship/internal/config"
	"relationship/internal/models"
	"relationship/internal/tasks"
)

// IngestService owns how a new record runs its AI pipeline: inline (the old
// behaviour, still available for tests and debugging) or through the
// background extraction queue (the default). Either way the fact row is stored
// first — the LLM never gates the user's text.
type IngestService struct {
	ai    *AIService
	queue *tasks.Queue // nil when running synchronously
}

// NewIngestService wires the coordinator. When async is enabled it starts the
// single extraction worker; Stop must be called on shutdown.
func NewIngestService(ai *AIService, cfg *config.LLMConfig) *IngestService {
	s := &IngestService{ai: ai}
	if cfg.AsyncExtract {
		s.queue = tasks.New("extract_event", func(ctx context.Context, eventID string) error {
			// RetryExtraction is the whole pipeline minus the create: extract,
			// store, index, refresh the profile. It is idempotent and refuses
			// to overwrite manual edits — exactly the semantics a queued run
			// wants. force stays false: a pending record is never hand-curated.
			_, _, err := ai.RetryExtraction(ctx, eventID, false)
			return err
		})
		s.queue.Start()
	}
	return s
}

// Async reports whether extraction runs through the queue.
func (s *IngestService) Async() bool { return s.queue != nil }

// Create stores a record and runs its pipeline. In async mode this returns as
// soon as the fact row exists, with a report that says the rest is queued; the
// worker then drives extraction, indexing and the profile refresh.
func (s *IngestService) Create(event *models.Event) (models.IngestReport, error) {
	if s.queue == nil {
		return s.ai.IngestEvent(context.Background(), event)
	}

	if err := s.ai.CreatePendingEvent(event); err != nil {
		return models.IngestReport{}, err
	}
	s.queue.Enqueue(event.ID)
	return models.IngestReport{Warnings: []string{}, Async: true}, nil
}

// RecoverPending re-enqueues records that were stored but never extracted —
// rows left pending by a crash, a shutdown with a full queue, or a pipeline
// that stopped between create and extract. It is the durability half of the
// async design: the queue is memory-only, the pending row is the truth.
func (s *IngestService) RecoverPending() (int, error) {
	if s.queue == nil {
		return 0, nil
	}
	events, err := s.ai.ListPendingEvents()
	if err != nil {
		return 0, err
	}
	for _, e := range events {
		s.queue.Enqueue(e.ID)
	}
	if len(events) > 0 {
		log.Printf("recovered %d pending record(s) into the extraction queue", len(events))
	}
	return len(events), nil
}

// TaskFor reports the latest queue task for an event, if any.
func (s *IngestService) TaskFor(eventID string) *tasks.Task {
	if s.queue == nil {
		return nil
	}
	return s.queue.Get(eventID)
}

// RecentTasks lists queue history for observability.
func (s *IngestService) RecentTasks(limit int) []*tasks.Task {
	if s.queue == nil {
		return []*tasks.Task{}
	}
	return s.queue.Snapshot(limit)
}

// Stats reports queue health: counters, current depth and run latency.
// Sync mode has no queue; the zero value with Async=false is the honest answer.
func (s *IngestService) Stats() tasks.Stats {
	if s.queue == nil {
		return tasks.Stats{}
	}
	return s.queue.Stats()
}

// Stop shuts the queue down: the in-flight extraction is cancelled, queued
// work is dropped, and the pending rows it leaves behind are what the next
// startup's RecoverPending picks up.
func (s *IngestService) Stop() {
	if s.queue != nil {
		s.queue.Stop()
	}
}
