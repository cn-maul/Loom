package repository

import (
	"testing"
)

// relation and notes are nullable without a default, and the AI-extracted event
// columns are NULL until the pipeline fills them in. Reading either used to fail
// with "converting NULL to string is unsupported".
func TestPersonReadsNullColumns(t *testing.T) {
	database := newTestDB(t)
	if _, err := database.Exec(`
		INSERT INTO persons (id, name, relation, notes, org_id, position)
		VALUES ('p-null', '无备注', NULL, NULL, NULL, NULL)`); err != nil {
		t.Fatal(err)
	}

	repo := NewPersonRepo(database)
	person, err := repo.GetByID("p-null")
	if err != nil {
		t.Fatalf("GetByID on a person with NULL columns must succeed, got %v", err)
	}
	if person.Relation != "" || person.Notes != "" || person.OrgID != "" || person.Position != "" {
		t.Fatalf("nullable columns should decode to empty values, got %+v", person)
	}

	list, err := repo.ListWithActivity()
	if err != nil {
		t.Fatalf("ListWithActivity must succeed, got %v", err)
	}
	found := false
	for _, p := range list {
		if p.ID == "p-null" {
			found = true
		}
	}
	if !found {
		t.Fatal("the person with NULL columns is missing from the list")
	}
}

func TestEventReadsNullColumns(t *testing.T) {
	database := newTestDB(t)
	if _, err := database.Exec(`
		INSERT INTO events (id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises)
		VALUES ('e-null', 'p1', '今天和张总开会', '2026-09-01', NULL, NULL, NULL, NULL)`); err != nil {
		t.Fatal(err)
	}

	repo := NewEventRepo(database)
	event, err := repo.GetByID("e-null")
	if err != nil {
		t.Fatalf("GetByID on an unextracted event must succeed, got %v", err)
	}
	if event.Summary != "" || event.MyFeeling != "" || event.TheirReaction != "" {
		t.Fatalf("nullable columns should decode to empty values, got %+v", event)
	}
	if event.RawText != "今天和张总开会" {
		t.Fatalf("raw_text is the source of truth and must survive, got %q", event.RawText)
	}

	all, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll must succeed, got %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 event, got %d", len(all))
	}
}
