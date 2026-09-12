package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"relationship/internal/db"
)

// openTestDB creates a real migrated database in a temp dir with one person
// and one record, so exports and backups carry recoverable content.
func openTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	database, err := db.OpenDB(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO persons (id, name, notes) VALUES ('p1', '张三', '私密备注')`); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO events (id, person_id, raw_text, event_date, summary)
		VALUES ('e1', 'p1', '他答应周五给数据', '2026-09-12', '承诺给数据')`); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	return database, path
}

func newSvc(t *testing.T, database *sql.DB, dbPath, dir string, interval time.Duration, keep int) *Service {
	t.Helper()
	return NewService(database, dbPath, dir, interval, keep, "")
}

func TestCreateBackupAndValidate(t *testing.T) {
	database, dbPath := openTestDB(t)
	svc := newSvc(t, database, dbPath, filepath.Join(t.TempDir(), "backups"), 0, 3)

	info, err := svc.Create()
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if !strings.HasPrefix(info.Name, filePrefix) || !strings.HasSuffix(info.Name, fileSuffix) {
		t.Fatalf("unexpected backup name: %s", info.Name)
	}
	if info.SizeBytes == 0 {
		t.Fatal("backup file is empty")
	}

	rep, err := Validate(svc.ResolvePath(info.Name))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !rep.OK {
		t.Fatalf("backup should validate, problems: %v", rep.Problems)
	}
	if rep.Integrity != "ok" {
		t.Fatalf("integrity = %q, want ok", rep.Integrity)
	}
	if rep.SchemaVersion == 0 {
		t.Fatal("schema version missing from backup")
	}
	if rep.ForeignKeyViolations != 0 {
		t.Fatalf("unexpected fk violations: %d", rep.ForeignKeyViolations)
	}

	list, err := svc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, err = %v", list, err)
	}
}

func TestValidateRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "loom-backup-bad.db")
	if err := os.WriteFile(bad, []byte("this is not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Validate(bad)
	if err != nil {
		t.Fatalf("validate returned error instead of report: %v", err)
	}
	if rep.OK {
		t.Fatal("garbage file must not validate")
	}
	if len(rep.Problems) == 0 {
		t.Fatal("expected problems to explain the rejection")
	}
}

func TestStageRestoreAndApplyRoundtrip(t *testing.T) {
	database, dbPath := openTestDB(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	svc := newSvc(t, database, dbPath, backupDir, 0, 3)

	info, err := svc.Create()
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}

	// Damage the live data after the snapshot: the restore must bring the
	// pre-damage state back.
	if _, err := database.Exec(`DELETE FROM events`); err != nil {
		t.Fatalf("damage data: %v", err)
	}

	if _, err := svc.StageRestore(info.Name); err != nil {
		t.Fatalf("stage restore: %v", err)
	}
	if _, err := os.Stat(dbPath + ".restore-pending"); err != nil {
		t.Fatalf("staged file missing: %v", err)
	}

	// Close before the swap, exactly like a real restart.
	database.Close()

	applied, err := ApplyPendingRestore(dbPath)
	if err != nil || !applied {
		t.Fatalf("apply pending restore: applied=%v err=%v", applied, err)
	}

	reopened, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	var count int
	if err := reopened.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("events after restore = %d, want 1 (pre-damage state)", count)
	}
}

func TestStageRestoreRejectsInvalidFile(t *testing.T) {
	database, dbPath := openTestDB(t)
	defer database.Close()
	svc := newSvc(t, database, dbPath, filepath.Join(t.TempDir(), "backups"), 0, 3)

	bad := filepath.Join(t.TempDir(), "bad.db")
	if err := os.WriteFile(bad, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := svc.StageRestore(bad)
	if err == nil {
		t.Fatal("staging a corrupt backup must fail")
	}
	if rep == nil || rep.OK {
		t.Fatalf("expected a failing report, got %+v", rep)
	}
	if _, err := os.Stat(dbPath + ".restore-pending"); !os.IsNotExist(err) {
		t.Fatal("no file may be staged from a corrupt backup")
	}
}

func TestExportFullAndRedacted(t *testing.T) {
	database, _ := openTestDB(t)

	full, err := Export(database, false)
	if err != nil {
		t.Fatalf("export full: %v", err)
	}
	fullText := string(full)
	for _, want := range []string{"张三", "他答应周五给数据", `"mode": "full"`, "event_participants"} {
		if !strings.Contains(fullText, want) {
			t.Fatalf("full export missing %q", want)
		}
	}

	red, err := Export(database, true)
	if err != nil {
		t.Fatalf("export redacted: %v", err)
	}
	redText := string(red)
	for _, leaked := range []string{"张三", "他答应周五给数据", "私密备注", "承诺给数据"} {
		if strings.Contains(redText, leaked) {
			t.Fatalf("redacted export leaks %q", leaked)
		}
	}
	for _, want := range []string{`"mode": "redacted"`, "[已脱敏]", "persons"} {
		if !strings.Contains(redText, want) {
			t.Fatalf("redacted export missing %q", want)
		}
	}
}

func TestPruneKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	// Five snapshots with distinct timestamped names; keep=2.
	for _, name := range []string{
		"loom-backup-20260101-000001.db",
		"loom-backup-20260102-000001.db",
		"loom-backup-20260103-000001.db",
		"loom-backup-20260104-000001.db",
		"loom-backup-20260105-000001.db",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc := newSvc(t, nil, filepath.Join(dir, "live.db"), dir, 0, 2)
	removed, err := svc.prune()
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 3 {
		t.Fatalf("pruned %d, want 3", removed)
	}
	left, _ := svc.List()
	if len(left) != 2 || left[0].Name != "loom-backup-20260105-000001.db" {
		t.Fatalf("kept %v, want the two newest", left)
	}
}
