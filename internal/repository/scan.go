package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"relationship/internal/models"
)

// timeArg renders an optional timestamp in the TEXT layout this schema stores,
// or NULL when it is absent.
func timeArg(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(models.SQLiteTimeLayout)
}

// timePointer turns a stored timestamp into a pointer, so an absent value stays
// absent in JSON instead of serialising as a zero time.
func timePointer(raw string) *time.Time {
	parsed := models.ParseSQLiteTime(raw)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so the row-shaping
// helpers below work for single lookups and for lists alike.
type rowScanner interface {
	Scan(dest ...any) error
}

// execer is satisfied by *sql.DB and *sql.Tx, which lets a write helper run either
// standalone or inside a transaction that also covers its side effects.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// scanNullString flattens a nullable TEXT column to the empty string the models
// expose. Scanning NULL straight into a string fails at runtime, so any column
// the schema may leave unset must come through here.
func scanNullString(s *sql.NullString) string {
	if !s.Valid {
		return ""
	}
	return strings.TrimSpace(s.String)
}

// requireRow turns a zero-row write into ErrNotFound. Without it a PUT, POST or
// DELETE against a missing id reported success.
func requireRow(res sql.Result, entity, id string) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read affected rows for %s %s: %w", entity, id, err)
	}
	if affected == 0 {
		return models.NewError(models.ErrNotFound, "%s %s not found", entity, id)
	}
	return nil
}

// inClause renders "?, ?, ..." for a list of ids and returns the query arguments
// in the same order.
func inClause(values []string) (string, []any) {
	if len(values) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		args[i] = v
	}
	return strings.Join(placeholders, ", "), args
}

// eventColumnsFor qualifies the shared column list with a table alias, so a query
// that joins events can still reuse one definition of the column order.
func eventColumnsFor(alias string) string {
	if alias == "" {
		return eventColumns
	}
	parts := strings.Split(eventColumns, ", ")
	for i, part := range parts {
		parts[i] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}
