package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"relationship/internal/models"
)

// A snapshot is written once and read whole: every section has to survive the
// JSON payload round trip, and the history row has to carry the headline
// without loading the payload.
func TestReportRepoRoundTrip(t *testing.T) {
	database := newTestDB(t)
	repo := NewReportRepo(database)

	report := &models.Report{
		ID:          uuid.New().String(),
		PersonID:    "p1",
		PersonName:  "张总",
		Start:       "2026-09-01",
		End:         "2026-09-07",
		Days:        7,
		Status:      models.ReportFailed,
		GeneratedBy: "local",
		// The degradation has to be visible in the history row, which reads the
		// failure_reason column rather than the payload.
		FailureReason: "model down",
		Summary:       "时间范围：2026-09-01 ~ 2026-09-07",
		Overdue: []models.ReportFollowUpRef{{
			ID: "f1", PersonID: "p1", PersonName: "张总", Title: "追答复",
			DueDate: "2026-09-01", DaysOverdue: 2,
		}},
		Events: []models.ReportEventRef{{
			ID: "e1", PersonID: "p1", PersonName: "张总",
			EventDate: "2026-09-02", Summary: "聊了合作", ExtractionStatus: "succeeded",
		}},
		Promises:   []models.ReportPromise{{EventID: "e1", Who: "张总", What: "下周给答复"}},
		EventCount: 1, PromiseCount: 1, OpenCount: 1,
	}
	if err := repo.Create(report); err != nil {
		t.Fatal(err)
	}

	stored, err := repo.Get(report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.ReportFailed || stored.PersonName != "张总" || stored.Days != 7 {
		t.Fatalf("stored headline = %+v", stored)
	}
	if len(stored.Overdue) != 1 || stored.Overdue[0].DaysOverdue != 2 {
		t.Fatalf("overdue section did not survive: %+v", stored.Overdue)
	}
	if len(stored.Events) != 1 || stored.Events[0].Summary != "聊了合作" {
		t.Fatalf("events section did not survive: %+v", stored.Events)
	}
	if len(stored.Promises) != 1 {
		t.Fatalf("promises section did not survive: %+v", stored.Promises)
	}
	if stored.GeneratedAt.IsZero() {
		t.Fatal("generated_at was not stamped")
	}

	// The history row is the list view: same headline fields, no payload needed.
	history, err := repo.List("", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != report.ID || history[0].FailureReason == "" {
		t.Fatalf("history = %+v", history)
	}
	if !history[0].GeneratedAt.After(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("history timestamp = %v", history[0].GeneratedAt)
	}

	if err := repo.Delete(report.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(report.ID); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("a deleted report = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(report.ID); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("double delete = %v, want ErrNotFound", err)
	}
}

// The person filter scopes the history, and rows come back newest first.
func TestReportRepoListFiltersByPerson(t *testing.T) {
	database := newTestDB(t)
	repo := NewReportRepo(database)

	// person_id is a real foreign key, so the history test needs real people.
	personRepo := NewPersonRepo(database)
	personIDs := map[string]string{}
	for _, name := range []string{"甲", "乙"} {
		person := &models.Person{ID: uuid.New().String(), Name: name}
		if err := personRepo.Create(person); err != nil {
			t.Fatal(err)
		}
		personIDs[name] = person.ID
	}

	make := func(personID string) {
		t.Helper()
		report := &models.Report{
			ID: uuid.New().String(), PersonID: personID, PersonName: "某人",
			Start: "2026-09-01", End: "2026-09-07", Days: 7,
			Status: models.ReportSucceeded, GeneratedBy: "local",
			Summary: "没有新的记录", EventCount: 0,
		}
		if err := repo.Create(report); err != nil {
			t.Fatal(err)
		}
	}
	p1 := personIDs["甲"]
	p2 := personIDs["乙"]
	make(p1)
	make(p2)
	make(p1)

	all, err := repo.List("", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("all = %d rows", len(all))
	}
	// Newest first. Timestamps have one-second granularity, so ties fall back
	// to rowid DESC, which is still insertion order.
	if all[0].PersonID != p1 || all[1].PersonID != p2 || all[2].PersonID != p1 {
		t.Fatalf("order = %s, %s, %s", all[0].PersonID, all[1].PersonID, all[2].PersonID)
	}

	scoped, err := repo.List(p2, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].PersonID != p2 {
		t.Fatalf("scoped = %+v", scoped)
	}
}
