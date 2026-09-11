package repository

import (
	"path/filepath"
	"testing"

	"relationship/internal/db"
	"relationship/internal/models"
)

// newPersonFilterDB seeds a throwaway database with persons spread across
// organisations, relations and importance levels, plus a couple of records so
// the activity ordering has something to chew on.
func newPersonFilterDB(t *testing.T) *PersonRepo {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "person_filter_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO organizations (id, name) VALUES ('o1', '腾讯')`,
		// created_at is pinned per person so the recent ordering (which falls
		// back to creation date for never-contacted persons) is deterministic.
		// 陈静: no org, high importance, notes mention "登山"
		`INSERT INTO persons (id, name, relation, importance, notes, created_at, updated_at)
		 VALUES ('p1', '陈静', '朋友', 5, '登山伙伴', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
		// 张总: org o1, 同事, one record 2026-09-01
		`INSERT INTO persons (id, name, relation, importance, org_id, position, created_at, updated_at)
		 VALUES ('p2', '张总', '同事', 3, 'o1', '总监', '2026-01-02 00:00:00', '2026-01-02 00:00:00')`,
		// 李工: org o1, 同事, one record 2026-09-10 (most recent)
		`INSERT INTO persons (id, name, relation, importance, org_id, position, created_at, updated_at)
		 VALUES ('p3', '李工', '同事', 2, 'o1', '工程师', '2026-01-03 00:00:00', '2026-01-03 00:00:00')`,
		// 王姐: no org, lowest importance, never contacted
		`INSERT INTO persons (id, name, relation, importance, created_at, updated_at)
		 VALUES ('p4', '王姐', '家人', 1, '2026-01-04 00:00:00', '2026-01-04 00:00:00')`,
		`INSERT INTO events (id, person_id, event_date, raw_text, created_at)
		 VALUES ('e1', 'p2', '2026-09-01', '和张总开会', datetime('now'))`,
		`INSERT INTO events (id, person_id, event_date, raw_text, created_at)
		 VALUES ('e2', 'p3', '2026-09-10', '和李工吃饭', datetime('now'))`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return NewPersonRepo(database)
}

func namesOf(persons []*models.PersonWithActivity) []string {
	names := make([]string, 0, len(persons))
	for _, person := range persons {
		names = append(names, person.Name)
	}
	return names
}

func sameNames(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestPersonRepoSearchHitsNotesAndPosition(t *testing.T) {
	repo := newPersonFilterDB(t)

	// The keyword reaches into notes, position and relation, not just the name.
	hits, total, err := repo.ListFiltered(models.PersonFilter{Q: "登山"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(hits) != 1 || hits[0].Name != "陈静" {
		t.Fatalf("notes search returned total=%d %v", total, namesOf(hits))
	}

	hits, total, err = repo.ListFiltered(models.PersonFilter{Q: "工程师"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(hits) != 1 || hits[0].Name != "李工" {
		t.Fatalf("position search returned total=%d %v", total, namesOf(hits))
	}

	hits, _, err = repo.ListFiltered(models.PersonFilter{Q: "同事"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("relation keyword should match both colleagues, got %v", namesOf(hits))
	}

	hits, total, err = repo.ListFiltered(models.PersonFilter{Q: "不存在的词"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || hits != nil {
		t.Fatalf("no-match search should return 0 and nil, got total=%d %v", total, hits)
	}
}

func TestPersonRepoOrgFilterSupportsNoneSentinel(t *testing.T) {
	repo := newPersonFilterDB(t)

	hits, total, err := repo.ListFiltered(models.PersonFilter{OrgID: "o1"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || !sameNames(namesOf(hits), "李工", "张总") {
		t.Fatalf("org filter returned total=%d %v", total, namesOf(hits))
	}

	hits, total, err = repo.ListFiltered(models.PersonFilter{OrgID: "none"})
	if err != nil {
		t.Fatal(err)
	}
	// No records, so the default recent ordering falls back to creation date,
	// newest first.
	if total != 2 || !sameNames(namesOf(hits), "王姐", "陈静") {
		t.Fatalf("none sentinel returned total=%d %v", total, namesOf(hits))
	}
}

func TestPersonRepoRelationFilterIsExact(t *testing.T) {
	repo := newPersonFilterDB(t)

	hits, total, err := repo.ListFiltered(models.PersonFilter{Relation: "朋友"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(hits) != 1 || hits[0].Name != "陈静" {
		t.Fatalf("relation filter returned total=%d %v", total, namesOf(hits))
	}

	// A partial relation string is a different value and must match nothing,
	// unlike the q keyword which is deliberately fuzzy.
	hits, total, err = repo.ListFiltered(models.PersonFilter{Relation: "朋"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || hits != nil {
		t.Fatalf("partial relation must not match, got total=%d %v", total, hits)
	}
}

func TestPersonRepoSortOrders(t *testing.T) {
	repo := newPersonFilterDB(t)

	// Default (recent): contacted persons by last record, then never-contacted
	// by creation date, newest first.
	hits, _, err := repo.ListFiltered(models.PersonFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if !sameNames(namesOf(hits), "李工", "张总", "王姐", "陈静") {
		t.Fatalf("recent order = %v", namesOf(hits))
	}

	hits, _, err = repo.ListFiltered(models.PersonFilter{Sort: models.PersonSortImportance})
	if err != nil {
		t.Fatal(err)
	}
	if !sameNames(namesOf(hits), "陈静", "张总", "李工", "王姐") {
		t.Fatalf("importance order = %v", namesOf(hits))
	}

	hits, _, err = repo.ListFiltered(models.PersonFilter{Sort: models.PersonSortName})
	if err != nil {
		t.Fatal(err)
	}
	// COLLATE NOCASE compares by code point, so Chinese names order by their
	// UTF-8 bytes; the assertion pins this deterministic behaviour.
	if !sameNames(namesOf(hits), "张总", "李工", "王姐", "陈静") {
		t.Fatalf("name order = %v", namesOf(hits))
	}

	// All four were inserted in one statement, so created_at ties; only the
	// query must not blow up and the total must stay right.
	hits, total, err := repo.ListFiltered(models.PersonFilter{Sort: models.PersonSortCreated})
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 || len(hits) != 4 {
		t.Fatalf("created order returned total=%d len=%d", total, len(hits))
	}
}

func TestPersonRepoPaginationCountsBeyondPage(t *testing.T) {
	repo := newPersonFilterDB(t)

	page, total, err := repo.ListFiltered(models.PersonFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || total != 4 {
		t.Fatalf("first page len=%d total=%d", len(page), total)
	}

	rest, total, err := repo.ListFiltered(models.PersonFilter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 2 || total != 4 {
		t.Fatalf("second page len=%d total=%d", len(rest), total)
	}

	// The two pages must not overlap.
	seen := map[string]bool{}
	for _, person := range append(append([]*models.PersonWithActivity{}, page...), rest...) {
		if seen[person.ID] {
			t.Fatalf("person %s appeared on two pages", person.ID)
		}
		seen[person.ID] = true
	}
}
