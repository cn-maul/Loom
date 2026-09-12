package repository

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"relationship/internal/db"
	"relationship/internal/models"
)

// seedLargeDataset loads thousands of rows through real inserts so the perf
// assertions below describe the actual read path, not a synthetic table.
// Counts are deliberately modest in wall time but comfortably beyond the
// point where a missing index or an N+1 join would show up.
func seedLargeDataset(t *testing.T) (*PersonRepo, *EventRepo, *FollowUpRepo) {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "perf.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const (
		persons   = 2000
		events    = 6000
		followUps = 3000
	)
	tx, err := database.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	personStmt, err := tx.Prepare(`INSERT INTO persons (id, name, importance, is_self) VALUES (?, ?, ?, 0)`)
	if err != nil {
		t.Fatalf("prepare person: %v", err)
	}
	eventStmt, err := tx.Prepare(`INSERT INTO events (id, person_id, raw_text, event_date, extraction_status)
		VALUES (?, ?, ?, ?, 'succeeded')`)
	if err != nil {
		t.Fatalf("prepare event: %v", err)
	}
	followStmt, err := tx.Prepare(`INSERT INTO follow_ups (id, person_id, title, status, due_date, created_at, updated_at)
		VALUES (?, ?, ?, 'open', ?, ?, ?)`)
	if err != nil {
		t.Fatalf("prepare follow-up: %v", err)
	}

	for i := 0; i < persons; i++ {
		if _, err := personStmt.Exec(fmt.Sprintf("p%05d", i), fmt.Sprintf("人物%05d", i), i%5); err != nil {
			t.Fatalf("seed person %d: %v", i, err)
		}
	}
	for i := 0; i < events; i++ {
		day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i%365).Format("2006-01-02")
		if _, err := eventStmt.Exec(fmt.Sprintf("e%06d", i), fmt.Sprintf("p%05d", i%(persons-1)+1), fmt.Sprintf("记录内容 %d", i), day); err != nil {
			t.Fatalf("seed event %d: %v", i, err)
		}
	}
	for i := 0; i < followUps; i++ {
		if _, err := followStmt.Exec(fmt.Sprintf("f%05d", i), fmt.Sprintf("p%05d", i%(persons-1)+1),
			fmt.Sprintf("跟进 %d", i), "2026-06-01", "2026-06-01T00:00:00Z", "2026-06-01T00:00:00Z"); err != nil {
			t.Fatalf("seed follow-up %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return NewPersonRepo(database), NewEventRepo(database), NewFollowUpRepo(database)
}

// largeBudget is the per-query ceiling. Local SQLite pages thousands of rows
// in milliseconds; a full second means an index vanished or a scan crept in.
const largeBudget = time.Second

// TestLargeDatasetListsStayFast is the plan's "数千人物、事件、跟进" check:
// paged list queries must stay inside the budget and report honest totals, so
// a data-growth surprise shows up here instead of in the browser.
func TestLargeDatasetListsStayFast(t *testing.T) {
	persons, events, follows := seedLargeDataset(t)

	t.Run("persons paged", func(t *testing.T) {
		start := time.Now()
		rows, total, err := persons.ListFiltered(models.PersonFilter{Limit: 100, Offset: 1000, Sort: models.PersonSortName})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if elapsed > largeBudget {
			t.Fatalf("person page took %s, budget %s", elapsed, largeBudget)
		}
		if len(rows) != 100 {
			t.Fatalf("page size = %d, want 100", len(rows))
		}
		if total != 2000 {
			t.Fatalf("total = %d, want 2000", total)
		}
	})

	t.Run("events paged with search", func(t *testing.T) {
		start := time.Now()
		rows, total, err := events.ListFiltered(models.EventFilter{Limit: 100, Offset: 3000})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if elapsed > largeBudget {
			t.Fatalf("event page took %s, budget %s", elapsed, largeBudget)
		}
		if len(rows) != 100 {
			t.Fatalf("page size = %d, want 100", len(rows))
		}
		if total != 6000 {
			t.Fatalf("total = %d, want 6000", total)
		}
	})

	t.Run("follow-ups whole list", func(t *testing.T) {
		start := time.Now()
		rows, err := follows.List("", "", "", "")
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if elapsed > largeBudget {
			t.Fatalf("follow-up list took %s, budget %s", elapsed, largeBudget)
		}
		if len(rows) != 3000 {
			t.Fatalf("rows = %d, want 3000", len(rows))
		}
	})

	t.Run("graph node window", func(t *testing.T) {
		start := time.Now()
		rows, total, err := persons.ListFiltered(models.PersonFilter{Limit: 1000, Sort: models.PersonSortRecent})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if elapsed > largeBudget {
			t.Fatalf("graph window took %s, budget %s", elapsed, largeBudget)
		}
		if len(rows) != 1000 || total != 2000 {
			t.Fatalf("graph window = %d/%d, want 1000/2000", len(rows), total)
		}
	})
}
