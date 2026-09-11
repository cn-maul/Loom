package service

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
)

type followUpFixture struct {
	svc    *FollowUpService
	person *models.Person
	db     *sql.DB
}

// newFollowUpFixture wires the follow-up service against a throwaway database
// migrated with the real schema.
func newFollowUpFixture(t *testing.T) *followUpFixture {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "follow_up_service_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	persons := NewPersonService(
		repository.NewPersonRepo(database),
		repository.NewVecRepo(database),
		repository.NewOrganizationRepo(database),
	)
	person := &models.Person{ID: uuid.New().String(), Name: "张总"}
	if err := persons.Create(person); err != nil {
		t.Fatal(err)
	}

	return &followUpFixture{
		svc: NewFollowUpService(
			repository.NewFollowUpRepo(database),
			persons,
			repository.NewEventRepo(database),
		),
		person: person,
		db:     database,
	}
}

// insertRecord writes a bare record for the cross-reference tests.
func (f *followUpFixture) insertRecord(t *testing.T, id string) {
	t.Helper()
	if _, err := f.db.Exec(
		`INSERT INTO events (id, person_id, raw_text, event_date) VALUES (?, ?, '开会', '2026-09-01')`,
		id, f.person.ID); err != nil {
		t.Fatal(err)
	}
}

func (f *followUpFixture) create(t *testing.T, title string) *models.FollowUp {
	t.Helper()
	item := &models.FollowUp{PersonID: f.person.ID, Title: title}
	if err := f.svc.Create(item); err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	return item
}

// The service defaulted status and left due_date empty; both must survive a
// create followed by a read.
func TestFollowUpCreateThenRead(t *testing.T) {
	f := newFollowUpFixture(t)
	created := f.create(t, "把合同发给张总")

	if created.Status != "pending" {
		t.Fatalf("status should default to pending, got %q", created.Status)
	}
	got, err := f.svc.Get(created.ID)
	if err != nil {
		t.Fatalf("Get after Create must succeed, got %v", err)
	}
	if got.Title != "把合同发给张总" || got.DueDate != "" || got.CompletedAt != nil {
		t.Fatalf("unexpected round trip: %+v", got)
	}

	items, err := f.svc.List(f.person.ID, "", "", "")
	if err != nil {
		t.Fatalf("List must include a pending item, got %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
}

func TestFollowUpMissingIDReturnsNotFound(t *testing.T) {
	f := newFollowUpFixture(t)
	ghost := uuid.New().String()

	if _, err := f.svc.Get(ghost); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound", err)
	}
	update := &models.FollowUp{ID: ghost, PersonID: f.person.ID, Title: "T"}
	if err := f.svc.Update(update); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Update = %v, want ErrNotFound", err)
	}
	if err := f.svc.Postpone(ghost, "2026-10-01", ""); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Postpone = %v, want ErrNotFound", err)
	}
	if err := f.svc.Action(ghost, "completed"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Action = %v, want ErrNotFound", err)
	}
}

func TestFollowUpUnknownPersonIsReported(t *testing.T) {
	f := newFollowUpFixture(t)
	item := &models.FollowUp{PersonID: uuid.New().String(), Title: "T"}
	if err := f.svc.Create(item); !errors.Is(err, models.ErrPersonNotFound) {
		t.Fatalf("Create with an unknown person = %v, want ErrPersonNotFound", err)
	}
}

func TestFollowUpCompleteIsIdempotent(t *testing.T) {
	f := newFollowUpFixture(t)
	item := f.create(t, "确认周五数据")

	if err := f.svc.Action(item.ID, "completed"); err != nil {
		t.Fatal(err)
	}
	first, err := f.svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "completed" || first.CompletedAt == nil {
		t.Fatalf("expected a completed item with a timestamp, got %+v", first)
	}

	if err := f.svc.Action(item.ID, "completed"); err != nil {
		t.Fatalf("repeating complete must succeed, got %v", err)
	}
	second, err := f.svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !second.CompletedAt.Equal(*first.CompletedAt) {
		t.Fatalf("completed_at changed on retry: %v -> %v", first.CompletedAt, second.CompletedAt)
	}
}

// Postponing moves the deadline and returns the item to pending. It is not the
// same action as waiting on the other side, and it must not reopen finished
// work.
func TestFollowUpPostponeSemantics(t *testing.T) {
	f := newFollowUpFixture(t)
	item := f.create(t, "催一下报价")

	if err := f.svc.Action(item.ID, "waiting"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Postpone(item.ID, "2026-10-01", "对方面谈时间待定"); err != nil {
		t.Fatal(err)
	}
	postponed, err := f.svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if postponed.Status != "pending" {
		t.Fatalf("postpone should return the item to pending, got %q", postponed.Status)
	}
	if postponed.DueDate != "2026-10-01" {
		t.Fatalf("due date = %q, want 2026-10-01", postponed.DueDate)
	}
	if postponed.CompletedAt != nil {
		t.Fatal("postponing must clear completed_at")
	}

	if err := f.svc.Action(item.ID, "completed"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Postpone(item.ID, "2026-11-01", ""); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("postponing a completed item = %v, want ErrConflict", err)
	}
}

func TestFollowUpRejectsBadInput(t *testing.T) {
	f := newFollowUpFixture(t)

	if err := f.svc.Postpone(uuid.New().String(), "10/01/2026", ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("malformed due date = %v, want ErrInvalidInput", err)
	}
	if _, err := f.svc.List("", "", "2026-9-1", ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("malformed range start = %v, want ErrInvalidInput", err)
	}
	if _, err := f.svc.List("", "", "2026-10-01", "2026-09-01"); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("inverted range = %v, want ErrInvalidInput", err)
	}
	if _, err := f.svc.List("", "done", "", ""); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("unknown status filter = %v, want ErrInvalidInput", err)
	}
	if err := f.svc.Create(&models.FollowUp{PersonID: f.person.ID}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("missing title = %v, want ErrInvalidInput", err)
	}
	if err := f.svc.Action(uuid.New().String(), "pending"); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("action to pending = %v, want ErrInvalidInput", err)
	}
}

func TestFollowUpListFiltersByDueRange(t *testing.T) {
	f := newFollowUpFixture(t)
	for _, spec := range []struct{ title, due string }{
		{"本期到期", "2026-09-12"},
		{"下期到期", "2026-10-05"},
	} {
		item := &models.FollowUp{PersonID: f.person.ID, Title: spec.title, DueDate: spec.due}
		if err := f.svc.Create(item); err != nil {
			t.Fatal(err)
		}
	}
	undated := f.create(t, "没有期限")

	items, err := f.svc.List("", "", "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "本期到期" {
		t.Fatalf("range filter returned %+v", items)
	}

	all, err := f.svc.List(f.person.ID, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 items, got %d", len(all))
	}
	if all[len(all)-1].ID != undated.ID {
		t.Fatalf("an item without a deadline should sort last, got %s", all[len(all)-1].Title)
	}
}

// The closed loop: an item remembers where it came from and what came of it.
func TestFollowUpCarriesSourceAndOutcome(t *testing.T) {
	f := newFollowUpFixture(t)
	f.insertRecord(t, "e1")

	item := &models.FollowUp{
		PersonID: f.person.ID, Title: "把报价单发给张总", SourceEventID: "e1",
		Owner: "我", DueText: "下周三前", DueDate: "2026-09-30",
	}
	if err := f.svc.Create(item); err != nil {
		t.Fatal(err)
	}

	got, err := f.svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceEventID != "e1" || got.Owner != "我" || got.DueText != "下周三前" {
		t.Fatalf("closure fields did not round trip: %+v", got)
	}

	if err := f.svc.Complete(item.ID, "张总已确认，等合同", "e1"); err != nil {
		t.Fatal(err)
	}
	done, err := f.svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != "completed" || done.CompletionNote != "张总已确认，等合同" || done.CompletedEventID != "e1" {
		t.Fatalf("outcome not recorded: %+v", done)
	}
}

// A dangling reference would make the item look traceable when it is not, so a
// source or outcome record that does not exist is refused before the write.
func TestFollowUpRejectsUnknownRecordReferences(t *testing.T) {
	f := newFollowUpFixture(t)
	ghost := uuid.New().String()

	item := &models.FollowUp{PersonID: f.person.ID, Title: "T", SourceEventID: ghost}
	if err := f.svc.Create(item); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("unknown source record = %v, want ErrNotFound", err)
	}

	f.insertRecord(t, "e1")
	ok := &models.FollowUp{PersonID: f.person.ID, Title: "T", SourceEventID: "e1"}
	if err := f.svc.Create(ok); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Complete(ok.ID, "完成了", ghost); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("unknown outcome record = %v, want ErrNotFound", err)
	}
	if err := f.svc.Update(&models.FollowUp{
		ID: ok.ID, PersonID: f.person.ID, Title: "T", SourceEventID: ghost,
	}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("update to an unknown source record = %v, want ErrNotFound", err)
	}
}

// An outcome is a property of finished work, so it cannot be written onto a new
// item through the create payload.
func TestFollowUpCreateIgnoresOutcomeFields(t *testing.T) {
	f := newFollowUpFixture(t)
	f.insertRecord(t, "e1")

	item := &models.FollowUp{
		PersonID: f.person.ID, Title: "T",
		CompletionNote: "不该被写入", CompletedEventID: "e1",
	}
	if err := f.svc.Create(item); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CompletionNote != "" || got.CompletedEventID != "" || got.Status != "pending" {
		t.Fatalf("an open item must not carry an outcome: %+v", got)
	}
}

func TestFollowUpExposesDeadlineHistory(t *testing.T) {
	f := newFollowUpFixture(t)
	item := f.create(t, "催一下报价")

	history, err := f.svc.ListPostponements(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("a new item has no history, got %d entries", len(history))
	}

	if err := f.svc.Postpone(item.ID, "2026-10-01", "对方出差"); err != nil {
		t.Fatal(err)
	}
	history, err = f.svc.ListPostponements(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Reason != "对方出差" {
		t.Fatalf("history = %+v", history)
	}

	if _, err := f.svc.ListPostponements(uuid.New().String()); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("history of an unknown item = %v, want ErrNotFound", err)
	}
}
