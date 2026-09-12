package backup

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// exportDocument is the on-disk shape of a JSON export. The table map is
// generic on purpose: it follows the live schema instead of hard-coding it,
// so adding a migration does not silently drop a table from future exports.
type exportDocument struct {
	Format        string                       `json:"format"`
	Version       int                          `json:"version"`
	Mode          string                       `json:"mode"`
	ExportedAt    string                       `json:"exported_at"`
	SchemaVersion int                          `json:"schema_version"`
	Tables        map[string][]map[string]any  `json:"tables"`
}

// redactColumns lists the columns a redacted export nulls out: everything
// that carries the actual words of a relationship — the raw record text, the
// feelings and reactions, profile notes, AI answers, report payloads. What
// survives is the shape of the data: counts, dates, ids, categories.
var redactColumns = map[string][]string{
	"events":                {"raw_text", "summary", "my_feeling", "their_reaction", "promises", "extraction_error"},
	"persons":               {"notes"},
	"traits":                {"trait_value"},
	"follow_ups":            {"title", "description", "completion_note", "due_text"},
	"advice_sessions":       {"question", "answer"},
	"report_snapshots":      {"summary", "payload", "failure_reason"},
	"person_relationships":  {"notes"},
	"person_org_positions":  {"notes"},
	"organizations":         {"description"},
}

// redactNames are identity columns replaced with a marker: a name is the most
// identifying field in the dataset.
var redactNames = map[string]string{
	"persons":       "name",
	"organizations": "name",
}

// preferredTableOrder puts the human-meaningful tables first in the export.
var preferredTableOrder = []string{
	"organizations", "persons", "events", "event_participants",
	"person_relationships", "person_org_positions", "traits",
	"follow_ups", "follow_up_postponements", "advice_sessions",
	"report_snapshots", "schema_migrations",
}

// Export dumps every application table to an indented JSON document. With
// redacted set, sensitive text columns are nulled and names replaced before
// anything is serialized — the document never contains the sensitive bytes.
func Export(db *sql.DB, redacted bool) ([]byte, error) {
	mode := "full"
	if redacted {
		mode = "redacted"
	}

	doc := exportDocument{
		Format:     "loom-export",
		Version:    1,
		Mode:       mode,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Tables:     map[string][]map[string]any{},
	}

	tables, err := listTables(db)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	if !sortTableOrder(tables) {
		sort.Strings(tables)
	}

	for _, table := range tables {
		rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s"`, table))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", table, err)
		}
		data, err := dumpRows(rows)
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("dump %s: %w", table, err)
		}
		if redacted {
			data = redact(table, data)
		}
		doc.Tables[table] = data
	}

	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&doc.SchemaVersion); err != nil {
		return nil, fmt.Errorf("read schema version: %w", err)
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode export: %w", err)
	}
	return out, nil
}

func dumpRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			v := values[i]
			// []byte arrives as a JSON-hostile blob; normalize to string.
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[c] = v
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// redact applies the per-table denylist. The redaction happens on the decoded
// row, so the sensitive content never reaches the serialized document.
func redact(table string, rows []map[string]any) []map[string]any {
	deny := map[string]bool{}
	for _, c := range redactColumns[table] {
		deny[c] = true
	}
	nameCol, hasName := redactNames[table]
	for _, row := range rows {
		for col := range row {
			if deny[col] {
				row[col] = nil
			}
		}
		if hasName {
			if _, ok := row[nameCol]; ok {
				row[nameCol] = "[已脱敏]"
			}
		}
	}
	return rows
}

// sortTableOrder applies preferredTableOrder and reports whether every table
// was covered (unknown tables keep schema order, appended at the end).
func sortTableOrder(tables []string) bool {
	rank := map[string]int{}
	for i, t := range preferredTableOrder {
		rank[t] = i
	}
	maxRank := len(preferredTableOrder)
	sort.SliceStable(tables, func(i, j int) bool {
		ri, oki := rank[tables[i]]
		if !oki {
			ri = maxRank
		}
		rj, okj := rank[tables[j]]
		if !okj {
			rj = maxRank
		}
		if ri == rj {
			return tables[i] < tables[j]
		}
		return ri < rj
	})
	return len(rank) >= len(tables)
}
