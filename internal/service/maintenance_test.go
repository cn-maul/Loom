package service

import (
	"path/filepath"
	"testing"

	"relationship/internal/db"
	"relationship/internal/repository"
)

func newMaintenanceFixture(t *testing.T, initVec bool) (*MaintenanceService, *repository.VecRepo) {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "maintenance_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	vec := repository.NewVecRepo(database)
	if initVec {
		if err := vec.Init(4); err != nil {
			t.Fatal(err)
		}
	}
	return NewMaintenanceService(database, vec, 0), vec
}

// seedMaintenanceFacts inserts the live rows the healthy vectors point at.
func seedMaintenanceFacts(t *testing.T, svc *MaintenanceService) {
	t.Helper()
	stmts := []string{
		`INSERT INTO persons (id, name) VALUES ('p1', '张总')`,
		`INSERT INTO events (id, person_id, raw_text, event_date) VALUES ('e1', 'p1', '开会', '2026-09-01')`,
		`INSERT INTO traits (id, person_id, trait_key, trait_value) VALUES ('t1', 'p1', '风格', '直接')`,
	}
	for _, q := range stmts {
		if _, err := svc.db.Exec(q); err != nil {
			t.Fatalf("seed: %v: %v", q, err)
		}
	}
}

func insertVec(t *testing.T, vec *repository.VecRepo, id, personID, chunkType, sourceID string) {
	t.Helper()
	if err := vec.Insert(id, []float32{1, 0, 0, 0}, personID, chunkType, sourceID); err != nil {
		t.Fatalf("insert vec %s: %v", id, err)
	}
}

func vecIDs(t *testing.T, svc *MaintenanceService) map[string]bool {
	t.Helper()
	rows, err := svc.db.Query(`SELECT id FROM vec_memory`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestCleanupRemovesOrphanVectors(t *testing.T) {
	svc, vec := newMaintenanceFixture(t, true)
	seedMaintenanceFacts(t, svc)

	// Live rows: every reference resolves.
	insertVec(t, vec, "event:e1", "p1", "event_summary", "e1")
	insertVec(t, vec, "trait:t1", "p1", "trait", "t1")
	// Orphans: source fact gone (the state a crash between a delete and its
	// index cleanup leaves behind).
	insertVec(t, vec, "event:gone", "p1", "event_summary", "missing-event")
	insertVec(t, vec, "trait:gone", "p1", "trait", "missing-trait")
	insertVec(t, vec, "event:pgone", "missing-person", "event_summary", "e1")
	// Unknown chunk type: a cleaner that does not understand it must not touch it.
	insertVec(t, vec, "future:1", "p1", "future_chunk", "whatever")

	report, err := svc.Cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if report.VectorsChecked != 6 || report.OrphanVectors != 3 {
		t.Fatalf("report = %+v, want checked=6 removed=3", report)
	}

	ids := vecIDs(t, svc)
	for _, keep := range []string{"event:e1", "trait:t1", "future:1"} {
		if !ids[keep] {
			t.Fatalf("live vector %s was removed", keep)
		}
	}
	for _, gone := range []string{"event:gone", "trait:gone", "event:pgone"} {
		if ids[gone] {
			t.Fatalf("orphan vector %s survived", gone)
		}
	}
}

func TestCleanupSkipsVectorsWhenIndexUnavailable(t *testing.T) {
	svc, _ := newMaintenanceFixture(t, false)

	report, err := svc.Cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if report.VectorsChecked != 0 || report.OrphanVectors != 0 {
		t.Fatalf("no index must mean zero checks, got %+v", report)
	}
}

func TestAuditRetentionTrimsOnlyExpiredRows(t *testing.T) {
	svc, _ := newMaintenanceFixture(t, true)
	stmts := []string{
		`INSERT INTO audit_log (id, action, path, status, created_at) VALUES ('a1', 'GET', '/api/persons', 200, '2020-01-01 00:00:00')`,
		`INSERT INTO audit_log (id, action, path, status, created_at) VALUES ('a2', 'GET', '/api/persons', 200, datetime('now'))`,
	}
	for _, q := range stmts {
		if _, err := svc.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	// Retention disabled: nothing is ever trimmed.
	svc.auditRetentionDays = 0
	report, err := svc.Cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if report.AuditRowsRemoved != 0 {
		t.Fatalf("retention 0 must keep everything, removed %+v", report)
	}

	// 30-day window: the 2020 row goes, today's row stays.
	svc.auditRetentionDays = 30
	report, err = svc.Cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if report.AuditRowsRemoved != 1 || report.AuditRetentionDays != 30 {
		t.Fatalf("report = %+v, want 1 removed", report)
	}
	var n int
	if err := svc.db.QueryRow(`SELECT count(*) FROM audit_log WHERE id = 'a1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("the expired audit row survived")
	}
}
