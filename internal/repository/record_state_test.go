package repository

import (
	"database/sql"
	"testing"

	"relationship/internal/models"
)

// insertEventRow writes a bare record, which is what a follow-up needs when it
// references the record it came out of.
func insertEventRow(t *testing.T, database *sql.DB, id, personID string) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO events (id, person_id, raw_text, event_date) VALUES (?, ?, 'T', '2026-09-01')`,
		id, personID); err != nil {
		t.Fatal(err)
	}
}

// A record is stored before the extractor runs, so it has to read back as
// "pending" rather than as an empty summary that looks like a result.
func TestEventRepoRoundTripsExtractionState(t *testing.T) {
	database := newTestDB(t)
	repo := NewEventRepo(database)

	event := &models.Event{ID: "e1", PersonID: "p1", RawText: "聊了合同", EventDate: "2026-09-01"}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(event); err != nil {
		t.Fatal(err)
	}

	stored, err := repo.GetByID("e1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ExtractionStatus != models.ExtractionPending {
		t.Fatalf("a fresh record should be pending, got %q", stored.ExtractionStatus)
	}
	if stored.ManuallyEdited != 0 || stored.ExtractedAt != nil || stored.EditedAt != nil {
		t.Fatalf("a fresh record should carry no edit state: %+v", stored)
	}

	stored.Summary = "聊了合同"
	if err := repo.UpdateExtraction(stored); err != nil {
		t.Fatal(err)
	}
	succeeded, err := repo.GetByID("e1")
	if err != nil {
		t.Fatal(err)
	}
	if succeeded.ExtractionStatus != models.ExtractionSucceeded || succeeded.ExtractedAt == nil {
		t.Fatalf("a successful extraction should be stamped, got %+v", succeeded)
	}

	// A failed retry reports the attempt but must not blank earlier content.
	if err := repo.RecordExtractionFailure("e1", "gateway down"); err != nil {
		t.Fatal(err)
	}
	failed, err := repo.GetByID("e1")
	if err != nil {
		t.Fatal(err)
	}
	if failed.ExtractionStatus != models.ExtractionFailed || failed.ExtractionError != "gateway down" {
		t.Fatalf("failure state = %q / %q", failed.ExtractionStatus, failed.ExtractionError)
	}
	if failed.Summary != "聊了合同" {
		t.Fatalf("a failed retry blanked the previous extraction: %q", failed.Summary)
	}
}

// A hand edit has to be visible, and a later extraction has to be able to clear
// it: the flag describes who wrote the stored content, not who touched the row.
func TestEventRepoTracksManualEdits(t *testing.T) {
	database := newTestDB(t)
	repo := NewEventRepo(database)
	insertEventRow(t, database, "e1", "p1")

	event, err := repo.GetByID("e1")
	if err != nil {
		t.Fatal(err)
	}
	event.Summary = "我自己写的摘要"
	if err := repo.Update(event); err != nil {
		t.Fatal(err)
	}

	edited, err := repo.GetByID("e1")
	if err != nil {
		t.Fatal(err)
	}
	if edited.ManuallyEdited != 1 || edited.EditedAt == nil {
		t.Fatalf("a hand edit should be flagged, got manually_edited=%d edited_at=%v",
			edited.ManuallyEdited, edited.EditedAt)
	}

	edited.Summary = "AI 摘要"
	if err := repo.UpdateExtraction(edited); err != nil {
		t.Fatal(err)
	}
	after, err := repo.GetByID("e1")
	if err != nil {
		t.Fatal(err)
	}
	if after.ManuallyEdited != 0 {
		t.Fatal("an extraction that overwrote the text leaves machine output behind, so the flag must clear")
	}
	if after.Summary != "AI 摘要" {
		t.Fatalf("summary = %q, want the extracted value", after.Summary)
	}
}

// The record list must be able to single out the records that need a retry.
func TestEventRepoFiltersByExtractionStatus(t *testing.T) {
	database := newTestDB(t)
	repo := NewEventRepo(database)
	insertEventRow(t, database, "e1", "p1")
	insertEventRow(t, database, "e2", "p1")
	if err := repo.RecordExtractionFailure("e2", "boom"); err != nil {
		t.Fatal(err)
	}

	failed, failedTotal, err := repo.ListFiltered(models.EventFilter{Status: models.ExtractionFailed, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].ID != "e2" {
		t.Fatalf("status filter returned %d rows", len(failed))
	}
	if failedTotal != 1 {
		t.Fatalf("failed total = %d, want 1", failedTotal)
	}

	pending, _, err := repo.ListFiltered(models.EventFilter{Status: models.ExtractionPending, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "e1" {
		t.Fatalf("status filter returned %d pending rows", len(pending))
	}
}

// A profile note whose evidence changed must be flagged, not silently dropped,
// and re-deriving it has to clear the flag again.
func TestTraitRepoMarksStaleSources(t *testing.T) {
	database := newTestDB(t)
	repo := NewTraitRepo(database)

	upsert := func(id, key, value string, sources ...string) {
		t.Helper()
		if err := repo.Upsert(&models.Trait{
			ID: id, PersonID: "p1", TraitKey: key, TraitValue: value,
			SourceEventIDs: models.EncodeIDList(sources),
		}); err != nil {
			t.Fatal(err)
		}
	}
	upsert("t1", "沟通风格", "直接", "e1")
	upsert("t2", "关注点", "预算", "e2")

	affected, err := repo.MarkSourceStale("e1", "来源记录已删除，画像需要重新计算")
	if err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("marking e1 stale affected %d rows, want 1", affected)
	}

	traits, err := repo.ListByPerson("p1")
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]*models.Trait{}
	for _, trait := range traits {
		byKey[trait.TraitKey] = trait
	}
	if byKey["沟通风格"].SourceStale != 1 || byKey["沟通风格"].SourceStaleReason == "" {
		t.Fatalf("the citing trait was not flagged: %+v", byKey["沟通风格"])
	}
	if byKey["关注点"].SourceStale != 0 {
		t.Fatal("a trait citing a different record must not be flagged")
	}

	// Marking the same source twice must not overwrite the original reason with
	// a later one, and re-deriving the note clears the marker.
	again, err := repo.MarkSourceStale("e1", "另一个原因")
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("a second marking changed %d already-stale rows", again)
	}
	upsert("t1", "沟通风格", "直接", "e1")
	refreshed, err := repo.GetByKey("p1", "沟通风格")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.SourceStale != 0 || refreshed.SourceStaleReason != "" {
		t.Fatalf("re-deriving the note should clear staleness, got %+v", refreshed)
	}
}
