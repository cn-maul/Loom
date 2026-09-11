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
// organisations the relationship, posting and participant tests hang off.
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

func seedRelationship(t *testing.T, repo *RelationshipRepo, id, from, to, relType string) *models.RelationshipLink {
	t.Helper()
	rel := &models.Relationship{ID: id, FromPersonID: from, ToPersonID: to, RelationType: relType, Confirmed: 1}
	if err := rel.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(rel); err != nil {
		t.Fatal(err)
	}
	link, err := repo.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return link
}

// A person's page shows both the edges they own and the ones pointing at them,
// and every edge carries the endpoint names for display.
func TestRelationshipRepoMatchesEitherEnd(t *testing.T) {
	repo := NewRelationshipRepo(newStructureDB(t))
	seedRelationship(t, repo, "r1", "p1", "p2", "上级")
	seedRelationship(t, repo, "r2", "p2", "p3", "同事")

	links, err := repo.List(models.RelationshipFilter{PersonID: "p2", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("person p2 should have 2 edges, got %d", len(links))
	}
	for _, link := range links {
		if link.FromPersonName == "" || link.ToPersonName == "" {
			t.Fatalf("endpoint names were not resolved: %+v", link)
		}
	}

	onlyColleagues, err := repo.List(models.RelationshipFilter{Type: "同事", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyColleagues) != 1 || onlyColleagues[0].ID != "r2" {
		t.Fatalf("type filter returned %+v", onlyColleagues)
	}
}

// An end_date closes an edge without deleting it, which is what keeps history
// readable.
func TestRelationshipRepoActiveFilterKeepsHistory(t *testing.T) {
	repo := NewRelationshipRepo(newStructureDB(t))
	rel := &models.Relationship{ID: "r1", FromPersonID: "p1", ToPersonID: "p2", RelationType: "同事"}
	if err := repo.Create(rel); err != nil {
		t.Fatal(err)
	}
	rel.EndDate = "2026-08-31"
	if err := repo.Update(rel); err != nil {
		t.Fatal(err)
	}

	active := true
	open, err := repo.List(models.RelationshipFilter{Active: &active, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("a closed edge must not appear as active, got %+v", open)
	}
	closed := false
	ended, err := repo.List(models.RelationshipFilter{Active: &closed, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(ended) != 1 {
		t.Fatal("a closed edge must still be listed as history")
	}
}

func TestRelationshipRepoMissingRowIsNotFound(t *testing.T) {
	repo := NewRelationshipRepo(newStructureDB(t))
	if _, err := repo.GetByID("ghost"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("GetByID(ghost) = %v, want ErrNotFound", err)
	}
	err := repo.Update(&models.Relationship{ID: "ghost", FromPersonID: "p1", ToPersonID: "p2", RelationType: "同事"})
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Update(ghost) = %v, want ErrNotFound", err)
	}
	if err := repo.Delete("ghost"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Delete(ghost) = %v, want ErrNotFound", err)
	}
}

// Shared experience is derived from attendance, never stored as a typed edge.
func TestRelationshipRepoCoAttendance(t *testing.T) {
	database := newStructureDB(t)
	repo := NewRelationshipRepo(database)
	for _, statement := range []string{
		`INSERT INTO events (id, person_id, raw_text, event_date) VALUES ('e1', 'p1', 'a', '2026-09-01')`,
		`INSERT INTO events (id, person_id, raw_text, event_date) VALUES ('e2', 'p1', 'b', '2026-09-02')`,
		`INSERT INTO events (id, person_id, raw_text, event_date) VALUES ('e3', 'p2', 'c', '2026-09-03')`,
		`INSERT INTO event_participants (event_id, person_id) VALUES ('e1', 'p1'), ('e1', 'p2')`,
		`INSERT INTO event_participants (event_id, person_id) VALUES ('e2', 'p1'), ('e2', 'p2'), ('e2', 'p3')`,
		`INSERT INTO event_participants (event_id, person_id) VALUES ('e3', 'p1'), ('e3', 'p2')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	pairs, err := repo.ListCoAttendance(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 3 {
		t.Fatalf("want 3 pairs, got %d (%+v)", len(pairs), pairs)
	}
	if pairs[0].PersonAName != "张总" || pairs[0].PersonBName != "李工" || pairs[0].SharedEvents != 3 {
		t.Fatalf("most-frequent pair = %+v, want 张总/李工 x3", pairs[0])
	}
}

func TestPositionRepoCurrentAndHistory(t *testing.T) {
	database := newStructureDB(t)
	repo := NewPositionRepo(database)

	current := &models.OrgPosition{ID: "pos1", PersonID: "p1", OrgID: "o1", Role: "总监"}
	if err := current.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(current); err != nil {
		t.Fatal(err)
	}
	past := &models.OrgPosition{ID: "pos2", PersonID: "p1", OrgID: "o2", Role: "顾问", StartDate: "2024-01-01", EndDate: "2025-12-31"}
	if err := past.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(past); err != nil {
		t.Fatal(err)
	}

	byPerson, err := repo.ListByPerson("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(byPerson) != 2 {
		t.Fatalf("want the full stint history, got %d", len(byPerson))
	}
	if byPerson[0].ID != "pos1" {
		t.Fatalf("the current posting must sort first, got %s", byPerson[0].ID)
	}
	if byPerson[0].PersonName != "张总" || byPerson[0].OrgName != "腾讯" {
		t.Fatalf("posting names were not resolved: %+v", byPerson[0])
	}

	onlyCurrent, err := repo.ListByOrg("o1", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyCurrent) != 1 {
		t.Fatalf("o1 should have 1 current member, got %d", len(onlyCurrent))
	}
	noCurrent, err := repo.ListByOrg("o2", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(noCurrent) != 0 {
		t.Fatalf("o2 has only a past member, got %d current", len(noCurrent))
	}
	ended, err := repo.ListByOrg("o2", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ended) != 1 {
		t.Fatal("the ended posting must remain as history")
	}

	if _, err := repo.GetByID("ghost"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("GetByID(ghost) = %v, want ErrNotFound", err)
	}
	err = repo.Update(&models.OrgPosition{ID: "ghost", PersonID: "p1", OrgID: "o1"})
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Update(ghost) = %v, want ErrNotFound", err)
	}
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
