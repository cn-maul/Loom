package backup

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

// Report is the result of checking one database file before it is trusted as
// a restore source. Every check degrades to a Problem entry instead of
// aborting, so the user sees the full picture in one round trip.
type Report struct {
	OK                   bool     `json:"ok"`
	Path                 string   `json:"path"`
	Integrity            string   `json:"integrity"`
	SchemaVersion        int      `json:"schema_version"`
	ForeignKeyViolations int      `json:"foreign_key_violations"`
	VecMemoryRows        *int64   `json:"vec_memory_rows"`
	Tables               []string `json:"tables"`
	Problems             []string `json:"problems"`
}

// coreTables must exist for a file to count as a Loom database.
var coreTables = []string{"persons", "events", "traits", "follow_ups"}

// Validate opens a candidate database file and checks the things a restore
// would silently trip over: page integrity, the migration version that the
// running build must be able to bring forward, referential integrity and the
// state of the vector table. A missing vec_memory is a warning, not a
// failure — the index is rebuildable and its absence must not block
// recovering the facts.
func Validate(path string) (*Report, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("stat backup file: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open backup file: %w", err)
	}
	defer db.Close()

	rep := &Report{Path: path, OK: true}

	// 1. Page-level integrity.
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		// A file that is not a database at all fails right here.
		rep.OK = false
		rep.Problems = append(rep.Problems, fmt.Sprintf("not a readable SQLite database: %v", err))
		return rep, nil
	}
	rep.Integrity = integrity
	if integrity != "ok" {
		rep.OK = false
		rep.Problems = append(rep.Problems, "integrity_check: "+integrity)
	}

	// 2. Core tables present.
	tables, err := listTables(db)
	if err != nil {
		rep.OK = false
		rep.Problems = append(rep.Problems, fmt.Sprintf("list tables failed: %v", err))
		return rep, nil
	}
	rep.Tables = tables
	have := map[string]bool{}
	for _, t := range tables {
		have[strings.ToLower(t)] = true
	}
	for _, t := range coreTables {
		if !have[t] {
			rep.OK = false
			rep.Problems = append(rep.Problems, "missing core table: "+t)
		}
	}

	// 3. Schema version: the live build must recognise the backup's schema.
	if have["schema_migrations"] {
		var version sql.NullInt64
		err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version)
		if err != nil || !version.Valid {
			rep.OK = false
			rep.Problems = append(rep.Problems, "schema version unreadable")
		} else {
			rep.SchemaVersion = int(version.Int64)
		}
	} else {
		rep.OK = false
		rep.Problems = append(rep.Problems, "missing schema_migrations table")
	}

	// 4. Referential integrity.
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM (SELECT 1 FROM pragma_foreign_key_check LIMIT 1)`).Scan(&rep.ForeignKeyViolations); err != nil {
		rep.OK = false
		rep.Problems = append(rep.Problems, fmt.Sprintf("foreign_key_check failed: %v", err))
	} else if rep.ForeignKeyViolations > 0 {
		rep.OK = false
		rep.Problems = append(rep.Problems, fmt.Sprintf("%d foreign key violation(s)", rep.ForeignKeyViolations))
	}

	// 5. Vector index state (informational).
	if have["vec_memory"] {
		var rows int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM vec_memory`).Scan(&rows); err != nil {
			rep.Problems = append(rep.Problems, "vec_memory unreadable — index will need a rebuild after restore")
		} else {
			rep.VecMemoryRows = &rows
		}
	} else {
		rep.Problems = append(rep.Problems, "vec_memory absent — run reindex after restore")
	}

	return rep, nil
}

func listTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'
		AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'vec_%' AND name NOT LIKE '\_%' ESCAPE '\'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// ValidateFile validates a snapshot, decrypting it first when the file is an
// encrypted snapshot. The decrypted copy lives in the system temp directory
// for the duration of the check only.
func (s *Service) ValidateFile(path string) (*Report, error) {
	if !looksEncrypted(path) {
		return Validate(path)
	}
	if s.passphrase == "" {
		return nil, errors.New("快照已加密，但 config.yaml 未配置 backup.passphrase，无法读取")
	}
	tmp, err := os.CreateTemp("", "loom-restore-*.db")
	if err != nil {
		return nil, fmt.Errorf("create temp for decrypt: %w", err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := decryptFile(path, tmp.Name(), s.passphrase); err != nil {
		return nil, err
	}
	return Validate(tmp.Name())
}

// StageRestore validates the chosen snapshot and, only if it is sound, copies
// it next to the live database as `<db>.restore-pending`. The staged file is
// always plaintext: after the swap the database must be directly openable.
// The swap itself happens in ApplyPendingRestore at the next startup, before
// any connection exists — that is what makes the restore atomic from the
// application's point of view.
func (s *Service) StageRestore(src string) (*Report, error) {
	path := s.ResolvePath(src)
	rep, err := s.ValidateFile(path)
	if err != nil {
		return nil, err
	}
	if !rep.OK {
		return rep, fmt.Errorf("backup failed validation: %s", strings.Join(rep.Problems, "; "))
	}

	if looksEncrypted(path) {
		// Decrypt straight into the staged location (via a temp file for an
		// atomic rename), so the pending file is a plain SQLite database.
		tmp, err := os.CreateTemp("", "loom-restore-*.db")
		if err != nil {
			return rep, fmt.Errorf("create temp for decrypt: %w", err)
		}
		tmp.Close()
		defer os.Remove(tmp.Name())
		if err := decryptFile(path, tmp.Name(), s.passphrase); err != nil {
			return rep, err
		}
		staged := s.dbPath + ".restore-pending"
		if err := copyFile(tmp.Name(), staged); err != nil {
			return rep, fmt.Errorf("stage restore file: %w", err)
		}
	} else {
		staged := s.dbPath + ".restore-pending"
		if err := copyFile(path, staged); err != nil {
			return rep, fmt.Errorf("stage restore file: %w", err)
		}
	}
	log.Printf("restore staged from %s; takes effect on next startup", path)
	return rep, nil
}

// ApplyPendingRestore performs the staged swap. Call it before opening the
// live database. A staged file that no longer validates is renamed aside
// instead of deleted: a rejected restore must stay diagnosable.
func ApplyPendingRestore(dbPath string) (bool, error) {
	staged := dbPath + ".restore-pending"
	if _, err := os.Stat(staged); err != nil {
		return false, nil
	}

	rep, err := Validate(staged)
	if err != nil || !rep.OK {
		rejected := staged + ".rejected"
		if rerr := os.Rename(staged, rejected); rerr != nil {
			rejected = staged
		}
		problem := fmt.Sprint(err)
		if rep != nil {
			problem = strings.Join(rep.Problems, "; ")
		}
		return false, fmt.Errorf("staged restore rejected (%s); kept at %s", problem, rejected)
	}

	// WAL sidecar files belong to the old database; leaving them behind would
	// corrupt the restored file on first open.
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("remove %s: %w", dbPath+suffix, err)
		}
	}
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("remove current database: %w", err)
	}
	if err := os.Rename(staged, dbPath); err != nil {
		return false, fmt.Errorf("activate restored database: %w", err)
	}
	return true, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// Rename within the same directory keeps the swap atomic.
	return os.Rename(tmp, dst)
}
