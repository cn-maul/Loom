package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"
)

func newVecRepo(t *testing.T) *VecRepo {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "vec.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewVecRepo(db)
}

func TestVecInsertAndSearch(t *testing.T) {
	r := newVecRepo(t)
	if err := r.Init(4); err != nil {
		t.Fatal(err)
	}

	embeddings := map[string][]float32{
		"e1": {1, 0, 0, 0},
		"e2": {0, 1, 0, 0},
		"e3": {0, 0, 1, 0},
	}
	for _, id := range []string{"e1", "e2", "e3"} {
		if err := r.Insert("event:"+id, embeddings[id], "p1", "event_summary", id); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
		if id != "e3" {
			if err := r.Insert("event:other-"+id, embeddings[id], "p2", "event_summary", "other"+id); err != nil {
				t.Fatalf("insert other person: %v", err)
			}
		}
	}

	distinct := []float32{0, 0, 0, 1}
	// Re-ingesting must replace by primary key, not accumulate stale vectors.
	if err := r.Insert("event:e1", distinct, "p1", "event_summary", "e1"); err != nil {
		t.Fatalf("re-insert: %v", err)
	}
	var vectors int
	if err := r.db.QueryRow(`SELECT count(*) FROM vec_memory WHERE source_id = 'e1'`).Scan(&vectors); err != nil {
		t.Fatal(err)
	}
	if vectors != 1 {
		t.Fatalf("re-ingest should keep one vector per source, got %d", vectors)
	}

	results, err := r.SearchByPerson(distinct, "p1", "event_summary", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].SourceID != "e1" {
		t.Fatalf("replaced vector should now be nearest, got %s", results[0].SourceID)
	}

	if err := r.DeleteBySourceID("e1"); err != nil {
		t.Fatal(err)
	}
	remaining, err := r.SearchByPerson([]float32{0, 0, 1, 0}, "p1", "event_summary", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 {
		t.Fatalf("p1 should still hold e2 and e3, got %+v", remaining)
	}
	if remaining[0].SourceID != "e3" {
		t.Fatalf("nearest after delete should be e3, got %s", remaining[0].SourceID)
	}
	for _, res := range remaining {
		if res.SourceID == "e1" {
			t.Fatalf("deleted source still returned: %+v", remaining)
		}
	}

	if err := r.DeleteByPerson("p1"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := r.db.QueryRow(`SELECT count(*) FROM vec_memory WHERE person_id = 'p1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("person delete left %d vectors behind", count)
	}
}

func TestVecInitRecreatesEmptyIndexAtNewDimension(t *testing.T) {
	r := newVecRepo(t)
	if err := r.Init(4); err != nil {
		t.Fatal(err)
	}
	if err := r.Init(8); err != nil {
		t.Fatalf("an unused index should follow the configured dimension, got %v", err)
	}
	dim, ok, err := r.dimension()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || dim != 8 {
		t.Fatalf("expected the table to be recreated at 8 dimensions, got %d (ok=%v)", dim, ok)
	}
}

func TestVecInitReportsDimensionMismatch(t *testing.T) {
	r := newVecRepo(t)
	if err := r.Init(4); err != nil {
		t.Fatal(err)
	}
	if err := r.Insert("event:e1", []float32{1, 0, 0, 0}, "p1", "event_summary", "e1"); err != nil {
		t.Fatal(err)
	}

	err := r.Init(8)
	if err == nil {
		t.Fatal("expected a dimension mismatch error, got nil")
	}
	t.Logf("mismatch reported: %v", err)

	var stored int
	if err := r.db.QueryRow(`SELECT count(*) FROM vec_memory`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("a refused Init must leave the populated index untouched, got %d rows", stored)
	}
}
