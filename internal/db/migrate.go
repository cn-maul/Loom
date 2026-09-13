package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// migration is one ordered, versioned schema change. A version is applied at most
// once and recorded in schema_migrations, so a database created by an earlier
// build is upgraded in place instead of being recreated.
type migration struct {
	version int
	name    string
	steps   []string
}

// newIDExpr mints a UUID-shaped key in SQL, so the backfill can assign ids
// without a round trip through Go.
const newIDExpr = `lower(hex(randomblob(4)) || '-' || hex(randomblob(2)) || '-' ||
	hex(randomblob(2)) || '-' || hex(randomblob(2)) || '-' || hex(randomblob(6)))`

var schemaMigrations = []migration{
	{
		version: 2,
		name:    "events keep a nullable primary person and record metadata",
		// The old events table declared person_id NOT NULL ... ON DELETE CASCADE,
		// so deleting a participant destroyed the record for everybody. SQLite
		// cannot alter a constraint, hence the table rebuild. record_type and
		// channel come along because they belong to the same record model.
		steps: []string{
			`PRAGMA foreign_keys = OFF`,
			`CREATE TABLE events_new (
				id TEXT PRIMARY KEY,
				person_id TEXT REFERENCES persons(id) ON DELETE SET NULL,
				raw_text TEXT NOT NULL,
				event_date TEXT NOT NULL,
				summary TEXT,
				my_feeling TEXT,
				their_reaction TEXT,
				promises TEXT DEFAULT '[]',
				record_type TEXT,
				channel TEXT,
				created_at TEXT DEFAULT (datetime('now'))
			)`,
			`INSERT INTO events_new (id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises, record_type, channel, created_at)
				SELECT id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises, NULL, NULL, created_at FROM events`,
			`DROP TABLE events`,
			`ALTER TABLE events_new RENAME TO events`,
			`CREATE INDEX IF NOT EXISTS idx_events_person_date ON events(person_id, event_date DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_events_date ON events(event_date DESC)`,
			`PRAGMA foreign_keys = ON`,
		},
	},
	{
		version: 3,
		name:    "structured relationships, organisation positions and event participants",
		steps: []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_persons_self ON persons(is_self) WHERE is_self = 1`,
			`CREATE TABLE IF NOT EXISTS person_relationships (
				id TEXT PRIMARY KEY,
				from_person_id TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
				to_person_id TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
				relation_type TEXT NOT NULL,
				direction TEXT NOT NULL DEFAULT 'directed',
				start_date TEXT,
				end_date TEXT,
				source_event_id TEXT REFERENCES events(id) ON DELETE SET NULL,
				confirmed INTEGER NOT NULL DEFAULT 0,
				notes TEXT,
				created_at TEXT DEFAULT (datetime('now')),
				updated_at TEXT DEFAULT (datetime('now'))
			)`,
			`CREATE INDEX IF NOT EXISTS idx_relationships_from ON person_relationships(from_person_id)`,
			`CREATE INDEX IF NOT EXISTS idx_relationships_to ON person_relationships(to_person_id)`,
			`CREATE TABLE IF NOT EXISTS person_org_positions (
				id TEXT PRIMARY KEY,
				person_id TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
				org_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
				role TEXT,
				start_date TEXT,
				end_date TEXT,
				source TEXT,
				notes TEXT,
				created_at TEXT DEFAULT (datetime('now')),
				updated_at TEXT DEFAULT (datetime('now'))
			)`,
			`CREATE INDEX IF NOT EXISTS idx_positions_person ON person_org_positions(person_id)`,
			`CREATE INDEX IF NOT EXISTS idx_positions_org ON person_org_positions(org_id)`,
			`CREATE TABLE IF NOT EXISTS event_participants (
				event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
				person_id TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
				role TEXT,
				created_at TEXT DEFAULT (datetime('now')),
				PRIMARY KEY (event_id, person_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_event_participants_person ON event_participants(person_id)`,
		},
	},
	{
		version: 4,
		name:    "backfill legacy org affiliation and single-person records",
		// Both statements are guarded by NOT EXISTS, so a re-run after a partial
		// failure cannot duplicate rows. Legacy start/end dates stay NULL: the old
		// archive never recorded them and inventing a date would be worse than an
		// explicit unknown.
		steps: []string{
			`INSERT INTO person_org_positions (id, person_id, org_id, role, start_date, end_date, source, notes)
				SELECT ` + newIDExpr + `, p.id, p.org_id, NULLIF(TRIM(COALESCE(p.position, '')), ''), NULL, NULL,
					'migration', '由旧档案 org_id 迁移，起止时间未知'
				FROM persons p
				WHERE p.org_id IS NOT NULL AND p.org_id <> ''
					AND NOT EXISTS (
						SELECT 1 FROM person_org_positions pos
						WHERE pos.person_id = p.id AND pos.org_id = p.org_id
					)`,
			`INSERT INTO event_participants (event_id, person_id, role, created_at)
				SELECT e.id, e.person_id, 'primary', COALESCE(e.created_at, datetime('now'))
				FROM events e
				WHERE e.person_id IS NOT NULL AND e.person_id <> ''
					AND NOT EXISTS (
						SELECT 1 FROM event_participants ep
						WHERE ep.event_id = e.id AND ep.person_id = e.person_id
					)`,
		},
	},
	{
		version: 5,
		name:    "follow-up closure, record extraction state and stale profile sources",
		// Three additions that all belong to "a record becomes an action and the
		// result is knowable again":
		//   follow_ups  gains its provenance, the owner, the original wording and
		//               the outcome, plus a table for the deadline history that
		//               an overwritten due_date used to lose.
		//   events      gains the extraction state, so a failed run is visible
		//               and retryable instead of looking like an empty summary,
		//               plus the manual-edit flag a retry must respect.
		//   traits      gains a stale marker, set when the record a profile note
		//               cites is edited or deleted.
		// ALTER TABLE ADD COLUMN only, so existing rows keep their data; the two
		// backfill statements are guarded so a re-run cannot double-apply them.
		steps: []string{
			`ALTER TABLE follow_ups ADD COLUMN source_event_id TEXT REFERENCES events(id) ON DELETE SET NULL`,
			`ALTER TABLE follow_ups ADD COLUMN owner TEXT`,
			`ALTER TABLE follow_ups ADD COLUMN due_text TEXT`,
			`ALTER TABLE follow_ups ADD COLUMN completion_note TEXT`,
			`ALTER TABLE follow_ups ADD COLUMN completed_event_id TEXT REFERENCES events(id) ON DELETE SET NULL`,
			`CREATE INDEX IF NOT EXISTS idx_follow_ups_source_event ON follow_ups(source_event_id)`,
			`CREATE TABLE IF NOT EXISTS follow_up_postponements (
				id TEXT PRIMARY KEY,
				follow_up_id TEXT NOT NULL REFERENCES follow_ups(id) ON DELETE CASCADE,
				old_due_date TEXT,
				new_due_date TEXT NOT NULL,
				reason TEXT,
				created_at TEXT DEFAULT (datetime('now'))
			)`,
			`CREATE INDEX IF NOT EXISTS idx_postponements_follow_up ON follow_up_postponements(follow_up_id, created_at)`,

			`ALTER TABLE events ADD COLUMN extraction_status TEXT`,
			`ALTER TABLE events ADD COLUMN extraction_error TEXT`,
			`ALTER TABLE events ADD COLUMN extracted_at TEXT`,
			`ALTER TABLE events ADD COLUMN manually_edited INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE events ADD COLUMN edited_at TEXT`,
			// Records written before this column existed were never tracked. One
			// with a summary did come back from the extractor; one without never
			// did, and calling that "failed" would invent a failure that was
			// never observed, so it stays pending.
			`UPDATE events SET extraction_status = 'pending' WHERE extraction_status IS NULL`,
			`UPDATE events SET extraction_status = 'succeeded', extracted_at = created_at
				WHERE extraction_status = 'pending'
					AND summary IS NOT NULL AND TRIM(summary) <> ''`,

			`ALTER TABLE traits ADD COLUMN source_stale INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE traits ADD COLUMN source_stale_reason TEXT`,
		},
	},
	{
		version: 6,
		name:    "persisted advice sessions with evidence versions",
		// Advice used to exist only as a response body: the moment the page was
		// reloaded the question, the model and the records behind it were gone,
		// so "why did I decide this?" was unanswerable. The session stores the
		// answer, the context actually used, and a content fingerprint of every
		// record it reasoned over — that fingerprint is what makes staleness a
		// comparison instead of a guess.
		// follow_ups gains its advice provenance. source_advice_id keeps the
		// ON DELETE SET NULL behaviour of source_event_id, and the two marker
		// columns are written *before* the session is removed, so deleting the
		// reasoning behind an action leaves a visible note rather than a
		// silently detached row.
		steps: []string{
			`CREATE TABLE IF NOT EXISTS advice_sessions (
				id TEXT PRIMARY KEY,
				person_id TEXT NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
				question TEXT NOT NULL,
				goal TEXT,
				answer TEXT NOT NULL DEFAULT '{}',
				used_event_ids TEXT NOT NULL DEFAULT '[]',
				used_trait_ids TEXT NOT NULL DEFAULT '[]',
				evidence_version TEXT NOT NULL DEFAULT '[]',
				retrieval_status TEXT,
				model TEXT,
				adopted_strategy_index INTEGER,
				adopted_strategy_name TEXT,
				created_at TEXT DEFAULT (datetime('now')),
				updated_at TEXT DEFAULT (datetime('now'))
			)`,
			`CREATE INDEX IF NOT EXISTS idx_advice_person ON advice_sessions(person_id, created_at DESC)`,

			`ALTER TABLE follow_ups ADD COLUMN source_advice_id TEXT REFERENCES advice_sessions(id) ON DELETE SET NULL`,
			`ALTER TABLE follow_ups ADD COLUMN source_advice_stale INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE follow_ups ADD COLUMN source_advice_stale_reason TEXT`,
			`CREATE INDEX IF NOT EXISTS idx_follow_ups_source_advice ON follow_ups(source_advice_id)`,
		},
	},
	{
		version: 7,
		name:    "report snapshots with an explicit window",
		// A report used to be a response body computed from "the last six days":
		// no window, no history, and its detail lines were pre-formatted
		// sentences with no ids to open. A snapshot makes the period explicit
		// and recoverable. The sections live in payload as one JSON document
		// because they are always read whole; the columns the history list
		// sorts on — window, status, headline — are real. person_name is
		// denormalized so a report stays readable after the person is deleted,
		// when the foreign key nulls person_id.
		steps: []string{
			`CREATE TABLE IF NOT EXISTS report_snapshots (
				id TEXT PRIMARY KEY,
				person_id TEXT REFERENCES persons(id) ON DELETE SET NULL,
				person_name TEXT,
				start_date TEXT NOT NULL,
				end_date TEXT NOT NULL,
				status TEXT NOT NULL DEFAULT 'succeeded',
				generated_by TEXT,
				failure_reason TEXT,
				summary TEXT,
				event_count INTEGER NOT NULL DEFAULT 0,
				payload TEXT NOT NULL DEFAULT '{}',
				generated_at TEXT DEFAULT (datetime('now'))
			)`,
			`CREATE INDEX IF NOT EXISTS idx_report_snapshots_window
				ON report_snapshots(start_date, end_date, generated_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_report_snapshots_person
				ON report_snapshots(person_id, generated_at DESC)`,
		},
	},
	{
		version: 8,
		name:    "organization metadata and archiving",
		// Organisations were bare name tags, so "which company is this and what
		// does it do" lived only in the user's head. Archiving is a soft state,
		// not deletion: a wound-down employer still explains every historical
		// posting that references it, and archived organisations simply stop
		// appearing in assignment pickers.
		steps: []string{
			`ALTER TABLE organizations ADD COLUMN kind TEXT`,
			`ALTER TABLE organizations ADD COLUMN description TEXT`,
			`ALTER TABLE organizations ADD COLUMN archived_at TEXT`,
		},
	},
	{
		version: 9,
		name:    "audit log for sensitive operations",
		// Exports, restores, deletes and index rebuilds change or expose the
		// whole dataset. One middleware writes them here, so "who did what to
		// my data" stays answerable even in a single-user local app.
		steps: []string{
			`CREATE TABLE IF NOT EXISTS audit_log (
				id TEXT PRIMARY KEY,
				action TEXT NOT NULL,
				path TEXT NOT NULL,
				status INTEGER NOT NULL,
				created_at TEXT DEFAULT (datetime('now'))
			)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at DESC)`,
		},
	},
	{
		version: 10,
		name:    "record pipeline warnings",
		// "Succeeded" used to hide partial work: a record whose vector indexing
		// or profile refresh failed looked identical to a fully processed one.
		// The warnings now ride on the row, so the list can show "succeeded
		// with warnings" instead of lying by omission.
		steps: []string{
			`ALTER TABLE events ADD COLUMN pipeline_warnings TEXT`,
		},
	},
}

// runMigrations applies the baseline schema, the legacy column additions and every
// pending versioned migration, then verifies referential integrity.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(migrationsSQL); err != nil {
		return fmt.Errorf("execute migrations: %w", err)
	}

	// CREATE TABLE IF NOT EXISTS never alters an existing table, so columns added
	// after a database was first created need an idempotent step here.
	for _, column := range []struct{ table, name, def string }{
		{"persons", "org_id", "TEXT REFERENCES organizations(id) ON DELETE SET NULL"},
		{"persons", "position", "TEXT"},
		{"persons", "is_self", "INTEGER NOT NULL DEFAULT 0"},
		{"persons", "gender", "TEXT"},
	} {
		if err := ensureColumn(db, column.table, column.name, column.def); err != nil {
			return err
		}
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range schemaMigrations {
		applied, err := migrationApplied(db, m.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return err
		}
	}

	return checkForeignKeys(db)
}

func migrationApplied(db *sql.DB, version int) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
		return false, fmt.Errorf("read schema_migrations: %w", err)
	}
	return count > 0, nil
}

// applyMigration runs a version's steps in order. Consecutive non-PRAGMA steps
// share one transaction, while PRAGMA statements run on their own connection
// state: `PRAGMA foreign_keys` is silently ignored inside a transaction, and the
// events rebuild needs it off around the table swap.
func applyMigration(db *sql.DB, m migration) error {
	var pending []string
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("migration %d begin: %w", m.version, err)
		}
		for _, step := range pending {
			if _, err := tx.Exec(step); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d (%s): %s: %w", m.version, m.name, statementLabel(step), err)
			}
		}
		pending = nil
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d commit: %w", m.version, err)
		}
		return nil
	}

	for _, step := range m.steps {
		if isPragma(step) {
			if err := flush(); err != nil {
				return err
			}
			if _, err := db.Exec(step); err != nil {
				return fmt.Errorf("migration %d (%s): %s: %w", m.version, m.name, step, err)
			}
			continue
		}
		pending = append(pending, step)
	}
	if err := flush(); err != nil {
		return err
	}

	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, m.version, m.name); err != nil {
		return fmt.Errorf("record migration %d: %w", m.version, err)
	}
	return nil
}

func isPragma(statement string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(statement)), "PRAGMA")
}

// statementLabel keeps the error readable without dumping a whole DDL block.
func statementLabel(statement string) string {
	label := strings.TrimSpace(statement)
	if i := strings.IndexByte(label, '\n'); i >= 0 {
		label = label[:i]
	}
	if len(label) > 80 {
		label = label[:80] + "..."
	}
	return label
}

// checkForeignKeys turns PRAGMA foreign_key_check into an error: after rebuilding
// a table it is the cheapest way to prove nothing was orphaned.
func checkForeignKeys(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("check foreign keys: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table, parent, fkid string
		var rowid sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return fmt.Errorf("scan foreign key violation: %w", err)
		}
		return fmt.Errorf("foreign key violation: table %s references %s", table, parent)
	}
	return rows.Err()
}
