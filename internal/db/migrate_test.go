package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := OpenDB(filepath.Join(t.TempDir(), "migrate_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func hasTable(t *testing.T, database *sql.DB, name string) bool {
	t.Helper()
	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count > 0
}

func hasColumn(t *testing.T, database *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := database.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	return false
}

func migrateVersions(t *testing.T, database *sql.DB) []int {
	t.Helper()
	rows, err := database.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, v)
	}
	return versions
}

func TestMigrateCreatesStructuredSchema(t *testing.T) {
	database := openTestDB(t)
	if err := Migrate(database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, table := range []string{
		"person_relationships", "person_org_positions", "event_participants", "schema_migrations",
		"follow_up_postponements", "advice_sessions",
	} {
		if !hasTable(t, database, table) {
			t.Fatalf("table %s was not created", table)
		}
	}
	if !hasColumn(t, database, "persons", "is_self") {
		t.Fatal("persons.is_self was not added")
	}
	for _, column := range []string{"record_type", "channel", "extraction_status", "extraction_error", "extracted_at", "manually_edited", "edited_at"} {
		if !hasColumn(t, database, "events", column) {
			t.Fatalf("events.%s was not added", column)
		}
	}
	for _, column := range []string{"source_event_id", "owner", "due_text", "completion_note", "completed_event_id",
		"source_advice_id", "source_advice_stale", "source_advice_stale_reason"} {
		if !hasColumn(t, database, "follow_ups", column) {
			t.Fatalf("follow_ups.%s was not added", column)
		}
	}
	for _, column := range []string{"source_stale", "source_stale_reason"} {
		if !hasColumn(t, database, "traits", column) {
			t.Fatalf("traits.%s was not added", column)
		}
	}
	for _, column := range []string{"question", "goal", "answer", "used_event_ids", "evidence_version",
		"retrieval_status", "model", "adopted_strategy_index"} {
		if !hasColumn(t, database, "advice_sessions", column) {
			t.Fatalf("advice_sessions.%s was not added", column)
		}
	}
	for _, column := range []string{"kind", "description", "archived_at"} {
		if !hasColumn(t, database, "organizations", column) {
			t.Fatalf("organizations.%s was not added", column)
		}
	}

	versions := migrateVersions(t, database)
	want := []int{2, 3, 4, 5, 6, 7, 8}
	if len(versions) != len(want) {
		t.Fatalf("schema_migrations = %v, want %v", versions, want)
	}
	for i, v := range want {
		if versions[i] != v {
			t.Fatalf("schema_migrations = %v, want %v", versions, want)
		}
	}
}

// Every start runs Migrate, so a second and third run must be silent no-ops.
func TestMigrateIsRepeatable(t *testing.T) {
	database := openTestDB(t)
	for run := 1; run <= 3; run++ {
		if err := Migrate(database); err != nil {
			t.Fatalf("Migrate run %d: %v", run, err)
		}
	}
	if versions := migrateVersions(t, database); len(versions) != 7 {
		t.Fatalf("schema_migrations grew on repeat runs: %v", versions)
	}
}

// Deleting an advice session must not take the follow-up that came out of it:
// the link is detached by the foreign key, but the item is work the user still
// has to do. The marker is written before the delete, which is why it has to be
// asserted here rather than trusted to the constraint.
func TestDeletingAdviceSessionKeepsFollowUpLink(t *testing.T) {
	database := openTestDB(t)
	if err := Migrate(database); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO persons (id, name) VALUES ('p1', '张总')`,
		`INSERT INTO advice_sessions (id, person_id, question) VALUES ('a1', 'p1', '怎么开口')`,
		`INSERT INTO follow_ups (id, person_id, title, status, source_advice_id)
			VALUES ('f1', 'p1', '约张总喝咖啡', 'pending', 'a1')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := database.Exec(
		`UPDATE follow_ups SET source_advice_stale = 1, source_advice_stale_reason = '来源建议已删除'
			WHERE source_advice_id = 'a1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DELETE FROM advice_sessions WHERE id = 'a1'`); err != nil {
		t.Fatalf("delete advice session: %v", err)
	}

	var stale int
	var reason sql.NullString
	var adviceID sql.NullString
	if err := database.QueryRow(
		`SELECT source_advice_stale, source_advice_stale_reason, source_advice_id FROM follow_ups WHERE id = 'f1'`).
		Scan(&stale, &reason, &adviceID); err != nil {
		t.Fatal(err)
	}
	if stale != 1 {
		t.Fatal("the follow-up must remember that its advice is gone")
	}
	if reason.String == "" {
		t.Fatal("the marker must explain what happened")
	}
	if adviceID.Valid {
		t.Fatalf("source_advice_id should have been detached, got %q", adviceID.String)
	}
}

// seedLegacyBaseline reproduces a database created before the versioned
// migrations existed: the original schema, with the old events constraints.
func seedLegacyBaseline(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(migrationsSQL); err != nil {
		t.Fatalf("apply baseline schema: %v", err)
	}
}

func TestMigrateBackfillsLegacyData(t *testing.T) {
	database := openTestDB(t)
	seedLegacyBaseline(t, database)
	for _, statement := range []string{
		`INSERT INTO organizations (id, name) VALUES ('o1', '腾讯')`,
		`INSERT INTO persons (id, name, org_id, position) VALUES ('p1', '张总', 'o1', '总监')`,
		`INSERT INTO events (id, person_id, raw_text, event_date) VALUES ('e1', 'p1', '和张总开会', '2026-09-01')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	if err := Migrate(database); err != nil {
		t.Fatalf("Migrate over legacy data: %v", err)
	}

	var orgID, role, startDate, endDate sql.NullString
	err := database.QueryRow(`SELECT org_id, role, start_date, end_date FROM person_org_positions WHERE person_id = 'p1'`).
		Scan(&orgID, &role, &startDate, &endDate)
	if err != nil {
		t.Fatalf("legacy org_id was not migrated to a posting: %v", err)
	}
	if orgID.String != "o1" || role.String != "总监" {
		t.Fatalf("posting = org %q role %q, want o1/总监", orgID.String, role.String)
	}
	if startDate.Valid || endDate.Valid {
		t.Fatal("legacy postings have unknown dates and must stay NULL, not be fabricated")
	}

	var participantRole string
	if err := database.QueryRow(
		`SELECT role FROM event_participants WHERE event_id = 'e1' AND person_id = 'p1'`).Scan(&participantRole); err != nil {
		t.Fatalf("legacy record was not migrated to a participant: %v", err)
	}
	if participantRole != "primary" {
		t.Fatalf("participant role = %q, want primary", participantRole)
	}

	// A re-run (for example after a crash mid-migration) must not duplicate rows.
	if err := Migrate(database); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var postings, participants int
	if err := database.QueryRow(`SELECT COUNT(*) FROM person_org_positions`).Scan(&postings); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM event_participants`).Scan(&participants); err != nil {
		t.Fatal(err)
	}
	if postings != 1 || participants != 1 {
		t.Fatalf("backfill duplicated rows: postings=%d participants=%d", postings, participants)
	}
}

// Records written before the extraction state existed were never tracked. One
// that already carries a summary did come back from the extractor; one without
// has to stay "pending" rather than be labelled a failure nobody observed.
func TestMigrateBackfillsExtractionState(t *testing.T) {
	database := openTestDB(t)
	seedLegacyBaseline(t, database)
	for _, statement := range []string{
		`INSERT INTO persons (id, name) VALUES ('p1', '张总')`,
		`INSERT INTO events (id, person_id, raw_text, event_date, summary, created_at)
			VALUES ('e1', 'p1', '开会', '2026-09-01', '聊了合同', '2026-09-01 10:00:00')`,
		`INSERT INTO events (id, person_id, raw_text, event_date) VALUES ('e2', 'p1', '随手记', '2026-09-02')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	if err := Migrate(database); err != nil {
		t.Fatalf("Migrate over legacy records: %v", err)
	}

	var status string
	var extractedAt sql.NullString
	if err := database.QueryRow(
		`SELECT extraction_status, extracted_at FROM events WHERE id = 'e1'`).Scan(&status, &extractedAt); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		t.Fatalf("a record with a summary should be succeeded, got %q", status)
	}
	if !extractedAt.Valid {
		t.Fatal("a migrated success should carry the time it was processed")
	}

	if err := database.QueryRow(`SELECT extraction_status FROM events WHERE id = 'e2'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("a record without a summary should stay pending, got %q", status)
	}

	// Re-running must not reclassify anything.
	if err := Migrate(database); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if err := database.QueryRow(`SELECT extraction_status FROM events WHERE id = 'e2'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("re-run changed e2 to %q", status)
	}
}

// Deleting a person used to cascade through events.person_id and destroy records
// other people also attended. The rebuilt table keeps them and just clears the
// primary person.
func TestDeletingPrimaryPersonKeepsSharedRecord(t *testing.T) {
	database := openTestDB(t)
	if err := Migrate(database); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO persons (id, name) VALUES ('p1', '张总')`,
		`INSERT INTO persons (id, name) VALUES ('p2', '李工')`,
		`INSERT INTO events (id, person_id, raw_text, event_date) VALUES ('e1', 'p1', '三方会议', '2026-09-02')`,
		`INSERT INTO event_participants (event_id, person_id) VALUES ('e1', 'p1'), ('e1', 'p2')`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := database.Exec(`DELETE FROM persons WHERE id = 'p1'`); err != nil {
		t.Fatal(err)
	}

	var events int
	if err := database.QueryRow(`SELECT COUNT(*) FROM events WHERE id = 'e1'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatal("a record two people attended must survive deleting one of them")
	}
	var personID sql.NullString
	if err := database.QueryRow(`SELECT person_id FROM events WHERE id = 'e1'`).Scan(&personID); err != nil {
		t.Fatal(err)
	}
	if personID.Valid {
		t.Fatalf("events.person_id should have been cleared, got %q", personID.String)
	}
	var remaining int
	if err := database.QueryRow(`SELECT COUNT(*) FROM event_participants WHERE event_id = 'e1'`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("the co-attendee must remain, got %d participant rows", remaining)
	}
}
