package repository

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"relationship/internal/db"
	"relationship/internal/models"
)

// newStructureDB returns a throwaway database holding the people and
// organisations the participant and person tests hang off.
func newStructureDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "structure_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO organizations (id, name) VALUES ('o1', '腾讯'), ('o2', '字节')`,
		`INSERT INTO persons (id, name) VALUES ('p1', '张总'), ('p2', '李工'), ('p3', '王姐')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

// One record, several attendees: it is stored once and read from each of them.
func TestEventRepoSharedRecordIsVisibleToEveryParticipant(t *testing.T) {
	database := newStructureDB(t)
	repo := NewEventRepo(database)

	event := &models.Event{
		ID: "e1", PersonID: "p1", RawText: "三方会议", EventDate: "2026-09-02",
		Participants: []models.EventParticipant{
			{PersonID: "p1"},
			{PersonID: "p2", Role: "同事"},
		},
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if event.Participants[0].Role != models.PrimaryParticipantRole {
		t.Fatalf("the anchored person must carry the primary role, got %q", event.Participants[0].Role)
	}
	if err := repo.Create(event); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID("e1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Participants) != 2 {
		t.Fatalf("want 2 participants, got %+v", got.Participants)
	}
	if got.Participants[0].PersonID != "p1" || got.Participants[0].PersonName != "张总" {
		t.Fatalf("the primary person must come first with a name, got %+v", got.Participants[0])
	}

	for _, personID := range []string{"p1", "p2"} {
		events, err := repo.ListByPerson(personID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 1 {
			t.Fatalf("person %s should see the shared record once, got %d", personID, len(events))
		}
	}
	uninvolved, err := repo.ListByPerson("p3", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(uninvolved) != 0 {
		t.Fatalf("an uninvolved person must not see the record, got %d", len(uninvolved))
	}

	// Replacing the list keeps the record itself untouched.
	if err := repo.SetParticipants("e1", []models.EventParticipant{{PersonID: "p3"}}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.GetByID("e1")
	if err != nil {
		t.Fatal(err)
	}
	if after.RawText != "三方会议" || len(after.Participants) != 1 || after.Participants[0].PersonID != "p3" {
		t.Fatalf("record or attendance diverged after the update: %+v", after)
	}
	if after.PersonID != "p3" {
		t.Fatalf("the primary person must follow the list, got %q", after.PersonID)
	}

	if err := repo.SetParticipants("e1", nil); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("emptying the attendance list = %v, want ErrInvalidInput", err)
	}
	if err := repo.SetParticipants("ghost", []models.EventParticipant{{PersonID: "p1"}}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("SetParticipants(ghost) = %v, want ErrNotFound", err)
	}
}

// The home page counts a record for everyone who attended it.
func TestPersonRepoCountsParticipatedEvents(t *testing.T) {
	database := newStructureDB(t)
	eventRepo := NewEventRepo(database)
	event := &models.Event{
		ID: "e1", PersonID: "p1", RawText: "会议", EventDate: "2026-09-02",
		Participants: []models.EventParticipant{{PersonID: "p1"}, {PersonID: "p2"}},
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := eventRepo.Create(event); err != nil {
		t.Fatal(err)
	}

	persons, err := NewPersonRepo(database).ListWithActivity()
	if err != nil {
		t.Fatal(err)
	}
	for _, person := range persons {
		if person.ID != "p2" {
			continue
		}
		if person.EventCount != 1 || person.LastEventDate != "2026-09-02" {
			t.Fatalf("participant activity = count %d date %q", person.EventCount, person.LastEventDate)
		}
		return
	}
	t.Fatal("person p2 was missing from the activity list")
}

// Exactly one person can be the user; promoting another demotes the previous one.
func TestPersonRepoSelfFlagIsExclusive(t *testing.T) {
	database := newStructureDB(t)
	repo := NewPersonRepo(database)

	first := &models.Person{ID: "self1", Name: "我", Importance: 3, IsSelf: 1}
	if err := repo.Create(first); err != nil {
		t.Fatal(err)
	}
	second := &models.Person{ID: "self2", Name: "我（新）", Importance: 3, IsSelf: 1}
	if err := repo.Create(second); err != nil {
		t.Fatal(err)
	}

	demoted, err := repo.GetByID("self1")
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := repo.GetByID("self2")
	if err != nil {
		t.Fatal(err)
	}
	if demoted.IsSelf != 0 || promoted.IsSelf != 1 {
		t.Fatalf("self flag not exclusive: first=%d second=%d", demoted.IsSelf, promoted.IsSelf)
	}
}
