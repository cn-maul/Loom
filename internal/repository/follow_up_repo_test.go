package repository

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"relationship/internal/db"
	"relationship/internal/models"
)

// newTestDB opens a throwaway database migrated with the real schema, so these
// tests never touch data/ or any developer database.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "follow_up_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO persons (id, name) VALUES ('p1', '张总')`); err != nil {
		t.Fatal(err)
	}
	return database
}

func insertRawFollowUp(t *testing.T, database *sql.DB, id, status, dueDate any) {
	t.Helper()
	_, err := database.Exec(`
		INSERT INTO follow_ups (id, person_id, title, description, due_date, status, completed_at)
		VALUES (?, 'p1', 'T', NULL, ?, ?, NULL)`, id, dueDate, status)
	if err != nil {
		t.Fatal(err)
	}
}

// A pending follow-up stores NULL for description, due_date and completed_at.
// Reading it must not fail, which is what the previous string scan did.
func TestFollowUpRepoReadsNullColumns(t *testing.T) {
	database := newTestDB(t)
	repo := NewFollowUpRepo(database)
	insertRawFollowUp(t, database, "f1", "pending", nil)

	got, err := repo.Get("f1")
	if err != nil {
		t.Fatalf("Get on a pending follow-up must succeed, got %v", err)
	}
	if got.Description != "" || got.DueDate != "" || got.CompletedAt != nil {
		t.Fatalf("nullable columns should decode to empty values, got %+v", got)
	}

	items, err := repo.List("", "", "", "")
	if err != nil {
		t.Fatalf("List with a pending row must succeed, got %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
}

func TestFollowUpRepoMissingRowIsNotFound(t *testing.T) {
	database := newTestDB(t)
	repo := NewFollowUpRepo(database)

	if _, err := repo.Get("ghost"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Get(ghost) = %v, want ErrNotFound", err)
	}
	err := repo.Update(&models.FollowUp{ID: "ghost", PersonID: "p1", Title: "T", Status: "pending"})
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Update(ghost) = %v, want ErrNotFound", err)
	}
	if err := repo.Postpone("ghost", "2026-10-01", ""); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Postpone(ghost) = %v, want ErrNotFound", err)
	}
	if err := repo.Action("ghost", "completed"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Action(ghost) = %v, want ErrNotFound", err)
	}
}

func TestFollowUpRepoOrdersUndatedLast(t *testing.T) {
	database := newTestDB(t)
	repo := NewFollowUpRepo(database)
	insertRawFollowUp(t, database, "undated", "pending", nil)
	insertRawFollowUp(t, database, "late", "pending", "2026-10-01")
	insertRawFollowUp(t, database, "soon", "pending", "2026-09-15")

	items, err := repo.List("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"soon", "late", "undated"}
	if len(items) != len(want) {
		t.Fatalf("want %d items, got %d", len(want), len(items))
	}
	for i, id := range want {
		if items[i].ID != id {
			t.Fatalf("position %d = %s, want %s", i, items[i].ID, id)
		}
	}
}

func TestFollowUpRepoActionKeepsCompletionConsistent(t *testing.T) {
	database := newTestDB(t)
	repo := NewFollowUpRepo(database)
	insertRawFollowUp(t, database, "f1", "pending", nil)

	if err := repo.Action("f1", "completed"); err != nil {
		t.Fatal(err)
	}
	completed, err := repo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	if completed.CompletedAt == nil {
		t.Fatal("completing a follow-up must stamp completed_at")
	}

	// Retrying the same action must not move the completion time.
	if err := repo.Action("f1", "completed"); err != nil {
		t.Fatalf("repeating complete must be a no-op, got %v", err)
	}
	again, err := repo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	if !again.CompletedAt.Equal(*completed.CompletedAt) {
		t.Fatalf("completed_at moved on retry: %v -> %v", completed.CompletedAt, again.CompletedAt)
	}
}

func TestFollowUpRepoActionsRejectFinishedItems(t *testing.T) {
	database := newTestDB(t)
	repo := NewFollowUpRepo(database)
	insertRawFollowUp(t, database, "done", "completed", "2026-09-01")
	insertRawFollowUp(t, database, "dropped", "cancelled", nil)

	if err := repo.Postpone("done", "2026-10-01", ""); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("postponing a completed follow-up = %v, want ErrConflict", err)
	}
	if err := repo.Postpone("dropped", "2026-10-01", ""); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("postponing a cancelled follow-up = %v, want ErrConflict", err)
	}
	if err := repo.Action("dropped", "completed"); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("completing a cancelled follow-up = %v, want ErrConflict", err)
	}
	if err := repo.Action("done", "cancelled"); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("cancelling a completed follow-up = %v, want ErrConflict", err)
	}
}

// Postponing used to overwrite the deadline with no record of what it replaced.
// The history is the only place the original date survives.
func TestFollowUpRepoRecordsDeadlineHistory(t *testing.T) {
	database := newTestDB(t)
	repo := NewFollowUpRepo(database)
	insertRawFollowUp(t, database, "f1", "pending", "2026-09-30")

	if err := repo.Postpone("f1", "2026-10-01", "对方出差"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Postpone("f1", "2026-10-15", ""); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	if got.DueDate != "2026-10-15" {
		t.Fatalf("due_date = %q, want the latest deadline", got.DueDate)
	}
	if len(got.Postponements) != 2 {
		t.Fatalf("want 2 history entries, got %d", len(got.Postponements))
	}
	first := got.Postponements[0]
	if first.OldDueDate != "2026-09-30" || first.NewDueDate != "2026-10-01" || first.Reason != "对方出差" {
		t.Fatalf("first entry = %+v", first)
	}
	if second := got.Postponements[1]; second.OldDueDate != "2026-10-01" || second.NewDueDate != "2026-10-15" {
		t.Fatalf("second entry = %+v", second)
	}

	history, err := repo.ListPostponements("f1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("ListPostponements returned %d entries", len(history))
	}
}

// An item without a deadline can still be postponed onto one; the history has to
// survive a NULL old value rather than fail the whole write.
func TestFollowUpRepoPostponesUndatedItem(t *testing.T) {
	database := newTestDB(t)
	repo := NewFollowUpRepo(database)
	insertRawFollowUp(t, database, "f1", "pending", nil)

	if err := repo.Postpone("f1", "2026-10-01", "先定个时间"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Postponements) != 1 || got.Postponements[0].OldDueDate != "" {
		t.Fatalf("history = %+v", got.Postponements)
	}
}

// Completing records what happened, not just that it did. Repeating the call
// must not move the completion time, but may still fill in a later outcome.
func TestFollowUpRepoCompleteRecordsOutcome(t *testing.T) {
	database := newTestDB(t)
	repo := NewFollowUpRepo(database)
	insertRawFollowUp(t, database, "f1", "pending", nil)
	insertEventRow(t, database, "e1", "p1")

	if err := repo.Complete("f1", "对方已确认", "e1"); err != nil {
		t.Fatal(err)
	}
	done, err := repo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != "completed" || done.CompletionNote != "对方已确认" || done.CompletedEventID != "e1" {
		t.Fatalf("outcome = %+v", done)
	}
	if done.CompletedAt == nil {
		t.Fatal("a completion must stamp completed_at")
	}
	stamp := *done.CompletedAt

	if err := repo.Complete("f1", "", ""); err != nil {
		t.Fatalf("repeating complete must be a no-op, got %v", err)
	}
	retried, err := repo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	if !retried.CompletedAt.Equal(stamp) {
		t.Fatalf("completed_at moved on retry: %v -> %v", stamp, retried.CompletedAt)
	}
	if retried.CompletionNote != "对方已确认" {
		t.Fatalf("the retry dropped the recorded outcome: %q", retried.CompletionNote)
	}

	if err := repo.Complete("f1", "补充：发票已开", ""); err != nil {
		t.Fatal(err)
	}
	annotated, err := repo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	if annotated.CompletionNote != "补充：发票已开" {
		t.Fatalf("a later outcome was not recorded: %q", annotated.CompletionNote)
	}
	if !annotated.CompletedAt.Equal(stamp) || annotated.CompletedEventID != "e1" {
		t.Fatal("adding an outcome must not rewrite the completion time or drop the linked record")
	}
}

// An edit that moves an item off "completed" has to clear the outcome, or the
// row claims a result for work that is open again.
func TestFollowUpRepoUpdateClearsOutcomeWhenReopened(t *testing.T) {
	database := newTestDB(t)
	repo := NewFollowUpRepo(database)
	insertRawFollowUp(t, database, "f1", "pending", nil)
	insertEventRow(t, database, "e1", "p1")

	if err := repo.Complete("f1", "已完成", "e1"); err != nil {
		t.Fatal(err)
	}
	item, err := repo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	item.Status = "pending"
	if err := repo.Update(item); err != nil {
		t.Fatal(err)
	}

	reopened, err := repo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.CompletedAt != nil || reopened.CompletionNote != "" || reopened.CompletedEventID != "" {
		t.Fatalf("reopening left outcome fields behind: %+v", reopened)
	}
}
