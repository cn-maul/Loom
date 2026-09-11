package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"relationship/internal/models"
)

type FollowUpRepo struct{ db *sql.DB }

func NewFollowUpRepo(db *sql.DB) *FollowUpRepo { return &FollowUpRepo{db: db} }

const followUpColumns = `id, person_id, title, description, owner, due_text, due_date, status,
	source_event_id, completion_note, completed_event_id, created_at, updated_at, completed_at,
	source_advice_id, source_advice_stale, source_advice_stale_reason`

// scanFollowUp reads one row. Every column except id, person_id, title and
// status is nullable in the schema, so they must go through sql.NullString:
// scanning a NULL into a plain string fails with "converting NULL to string is
// unsupported", which broke every read of a not-yet-completed follow-up.
func scanFollowUp(row rowScanner) (*models.FollowUp, error) {
	f := &models.FollowUp{}
	var description, owner, dueText, dueDate, sourceEvent, completionNote, completedEvent sql.NullString
	var sourceAdvice, sourceAdviceReason sql.NullString
	var created, updated, completed sql.NullString
	if err := row.Scan(
		&f.ID, &f.PersonID, &f.Title, &description, &owner, &dueText, &dueDate, &f.Status,
		&sourceEvent, &completionNote, &completedEvent, &created, &updated, &completed,
		&sourceAdvice, &f.SourceAdviceStale, &sourceAdviceReason,
	); err != nil {
		return nil, err
	}
	f.Description = scanNullString(&description)
	f.Owner = scanNullString(&owner)
	f.DueText = scanNullString(&dueText)
	f.DueDate = scanNullString(&dueDate)
	f.SourceEventID = scanNullString(&sourceEvent)
	f.CompletionNote = scanNullString(&completionNote)
	f.CompletedEventID = scanNullString(&completedEvent)
	f.SourceAdviceID = scanNullString(&sourceAdvice)
	f.SourceAdviceStaleReason = scanNullString(&sourceAdviceReason)
	f.CreatedAt = models.ParseSQLiteTime(scanNullString(&created))
	f.UpdatedAt = models.ParseSQLiteTime(scanNullString(&updated))
	// An open item stores NULL here; only a real timestamp becomes a pointer, so
	// the JSON stays free of a zero-time placeholder.
	if completedAt := models.ParseSQLiteTime(scanNullString(&completed)); !completedAt.IsZero() {
		f.CompletedAt = &completedAt
	}
	return f, nil
}

func (r *FollowUpRepo) Create(f *models.FollowUp) error {
	now, t := models.NowUTC()
	_, err := r.db.Exec(`
		INSERT INTO follow_ups (id, person_id, title, description, owner, due_text, due_date, status,
			source_event_id, source_advice_id, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		f.ID, f.PersonID, f.Title, nullIfEmpty(f.Description), nullIfEmpty(f.Owner),
		nullIfEmpty(f.DueText), nullIfEmpty(f.DueDate), f.Status,
		nullIfEmpty(f.SourceEventID), nullIfEmpty(f.SourceAdviceID), now, now,
	)
	if err != nil {
		return fmt.Errorf("create follow-up: %w", err)
	}
	f.CreatedAt, f.UpdatedAt = t, t
	return nil
}

// MarkSourceAdviceStale flags every follow-up created from an advice session that
// has gone away. Unlike source_event_id, which is simply detached, this leaves a
// note on the open item: the action was decided on reasoning, and finding out
// that the reasoning is gone changes what the item means. Only unmarked rows are
// touched, so a second pass cannot overwrite the original explanation.
func (r *FollowUpRepo) MarkSourceAdviceStale(adviceID, reason string) (int, error) {
	res, err := r.db.Exec(`
		UPDATE follow_ups SET source_advice_stale = 1, source_advice_stale_reason = ?
		WHERE source_advice_stale = 0 AND source_advice_id = ?`, reason, adviceID)
	if err != nil {
		return 0, fmt.Errorf("mark follow-ups stale for advice %s: %w", adviceID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read affected follow-ups: %w", err)
	}
	return int(affected), nil
}

// ListIDsByAdvice returns the follow-ups created from each given advice session,
// keyed by session id. Batching keeps an advice history of N rows to one query
// instead of N.
func (r *FollowUpRepo) ListIDsByAdvice(adviceIDs []string) (map[string][]string, error) {
	grouped := make(map[string][]string, len(adviceIDs))
	for _, id := range adviceIDs {
		grouped[id] = []string{}
	}
	if len(adviceIDs) == 0 {
		return grouped, nil
	}
	placeholders, args := inClause(adviceIDs)
	rows, err := r.db.Query(`
		SELECT source_advice_id, id FROM follow_ups
		WHERE source_advice_id IN (`+placeholders+`)
		ORDER BY created_at, rowid`, args...)
	if err != nil {
		return nil, fmt.Errorf("list follow-ups by advice: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var adviceID, id string
		if err := rows.Scan(&adviceID, &id); err != nil {
			return nil, fmt.Errorf("scan follow-up id: %w", err)
		}
		grouped[adviceID] = append(grouped[adviceID], id)
	}
	return grouped, rows.Err()
}

// Get reads one item together with its deadline history. The list path leaves
// Postponements empty rather than paying one query per row.
func (r *FollowUpRepo) Get(id string) (*models.FollowUp, error) {
	f, err := scanFollowUp(r.db.QueryRow(`SELECT `+followUpColumns+` FROM follow_ups WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.NewError(models.ErrNotFound, "follow-up %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get follow-up: %w", err)
	}
	history, err := r.ListPostponements(id)
	if err != nil {
		return nil, err
	}
	f.Postponements = history
	return f, nil
}

func (r *FollowUpRepo) List(personID, status, from, to string) ([]*models.FollowUp, error) {
	query := `SELECT ` + followUpColumns + ` FROM follow_ups WHERE 1 = 1`
	args := []interface{}{}
	if personID != "" {
		query += ` AND person_id = ?`
		args = append(args, personID)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	if from != "" {
		query += ` AND due_date >= ?`
		args = append(args, from)
	}
	if to != "" {
		query += ` AND due_date <= ?`
		args = append(args, to)
	}
	// COALESCE keeps rows without a deadline together at the end; a bare
	// `due_date = ''` test is NULL for those rows and would sort them first.
	query += ` ORDER BY CASE WHEN COALESCE(due_date, '') = '' THEN 1 ELSE 0 END, due_date, created_at`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list follow-ups: %w", err)
	}
	defer rows.Close()

	result := []*models.FollowUp{}
	for rows.Next() {
		f, err := scanFollowUp(rows)
		if err != nil {
			return nil, fmt.Errorf("scan follow-up: %w", err)
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

// ListPostponements returns the deadline history, oldest first.
func (r *FollowUpRepo) ListPostponements(followUpID string) ([]models.Postponement, error) {
	rows, err := r.db.Query(`
		SELECT id, follow_up_id, old_due_date, new_due_date, reason, created_at
		FROM follow_up_postponements WHERE follow_up_id = ?
		ORDER BY created_at, rowid`, followUpID)
	if err != nil {
		return nil, fmt.Errorf("list postponements: %w", err)
	}
	defer rows.Close()

	history := []models.Postponement{}
	for rows.Next() {
		var p models.Postponement
		var oldDue, reason, created sql.NullString
		if err := rows.Scan(&p.ID, &p.FollowUpID, &oldDue, &p.NewDueDate, &reason, &created); err != nil {
			return nil, fmt.Errorf("scan postponement: %w", err)
		}
		p.OldDueDate = scanNullString(&oldDue)
		p.Reason = scanNullString(&reason)
		p.CreatedAt = models.ParseSQLiteTime(scanNullString(&created))
		history = append(history, p)
	}
	return history, rows.Err()
}

// Update applies an explicit edit. completed_at and the outcome fields follow
// the status: they are stamped when an edit moves the item to completed, kept
// if it was already there, and cleared for every other status so the columns
// never disagree with the status they belong to.
func (r *FollowUpRepo) Update(f *models.FollowUp) error {
	now, _ := models.NowUTC()
	res, err := r.db.Exec(`
		UPDATE follow_ups SET title = ?, description = ?, owner = ?, due_text = ?, due_date = ?, status = ?,
			completed_at = CASE WHEN ? = 'completed' THEN COALESCE(completed_at, ?) ELSE NULL END,
			completion_note = CASE WHEN ? = 'completed' THEN ? ELSE NULL END,
			completed_event_id = CASE WHEN ? = 'completed' THEN ? ELSE NULL END,
			updated_at = ?
		WHERE id = ?`,
		f.Title, nullIfEmpty(f.Description), nullIfEmpty(f.Owner), nullIfEmpty(f.DueText),
		nullIfEmpty(f.DueDate), f.Status,
		f.Status, now,
		f.Status, nullIfEmpty(f.CompletionNote),
		f.Status, nullIfEmpty(f.CompletedEventID),
		now, f.ID,
	)
	if err != nil {
		return fmt.Errorf("update follow-up: %w", err)
	}
	return requireRow(res, "follow-up", f.ID)
}

// Complete closes an item and records what actually happened. An already
// completed item is not reopened and its completion time does not move; the
// call may still fill in an outcome the caller learned afterwards, because
// that is new information rather than a contradiction.
func (r *FollowUpRepo) Complete(id, completionNote, completedEventID string) error {
	current, err := r.statusOf(id)
	if err != nil {
		return err
	}
	if current == models.FollowUpCancelled {
		return models.NewError(models.ErrConflict, "cannot complete a cancelled follow-up")
	}

	now, _ := models.NowUTC()
	if current == models.FollowUpCompleted {
		if completionNote == "" && completedEventID == "" {
			return nil
		}
		res, err := r.db.Exec(`
			UPDATE follow_ups SET
				completion_note = COALESCE(NULLIF(?, ''), completion_note),
				completed_event_id = COALESCE(NULLIF(?, ''), completed_event_id),
				updated_at = ?
			WHERE id = ?`, completionNote, completedEventID, now, id)
		if err != nil {
			return fmt.Errorf("record follow-up outcome: %w", err)
		}
		return requireRow(res, "follow-up", id)
	}

	res, err := r.db.Exec(`
		UPDATE follow_ups SET status = 'completed', completed_at = ?,
			completion_note = ?, completed_event_id = ?, updated_at = ?
		WHERE id = ?`,
		now, nullIfEmpty(completionNote), nullIfEmpty(completedEventID), now, id)
	if err != nil {
		return fmt.Errorf("complete follow-up: %w", err)
	}
	return requireRow(res, "follow-up", id)
}

// Postpone moves the deadline and returns the item to pending work, appending
// the move to the deadline history. Postponing is not the same business meaning
// as waiting on the other side, so status never becomes waiting here, and a
// completed or cancelled item is not silently reopened.
func (r *FollowUpRepo) Postpone(id, dueDate, reason string) error {
	current, err := r.statusOf(id)
	if err != nil {
		return err
	}
	if current == models.FollowUpCompleted || current == models.FollowUpCancelled {
		return models.NewError(models.ErrConflict, "cannot postpone a %s follow-up", current)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin postpone: %w", err)
	}
	defer tx.Rollback()

	var previous sql.NullString
	if err := tx.QueryRow(`SELECT due_date FROM follow_ups WHERE id = ?`, id).Scan(&previous); err != nil {
		return fmt.Errorf("read previous deadline: %w", err)
	}

	now, _ := models.NowUTC()
	res, err := tx.Exec(`
		UPDATE follow_ups SET due_date = ?, status = 'pending', completed_at = NULL,
			completion_note = NULL, completed_event_id = NULL, updated_at = ?
		WHERE id = ?`, dueDate, now, id)
	if err != nil {
		return fmt.Errorf("postpone follow-up: %w", err)
	}
	if err := requireRow(res, "follow-up", id); err != nil {
		return err
	}
	// The history row is what keeps the original deadline recoverable; without
	// it a postpone silently replaces the date it replaced.
	if _, err := tx.Exec(`
		INSERT INTO follow_up_postponements (id, follow_up_id, old_due_date, new_due_date, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), id, nullIfEmpty(scanNullString(&previous)), dueDate, nullIfEmpty(reason), now); err != nil {
		return fmt.Errorf("record postponement: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit postpone: %w", err)
	}
	return nil
}

// Action applies a status change and keeps completed_at consistent with it.
// Repeating the same action is a no-op, so a retried request neither moves the
// completion time nor fails.
func (r *FollowUpRepo) Action(id, status string) error {
	if status == models.FollowUpCompleted {
		return r.Complete(id, "", "")
	}
	current, err := r.statusOf(id)
	if err != nil {
		return err
	}
	if current == status {
		return nil
	}
	if current == models.FollowUpCompleted || current == models.FollowUpCancelled {
		return models.NewError(models.ErrConflict, "cannot move a %s follow-up to %s", current, status)
	}

	now, _ := models.NowUTC()
	res, err := r.db.Exec(`
		UPDATE follow_ups SET status = ?, completed_at = NULL, completion_note = NULL,
			completed_event_id = NULL, updated_at = ?
		WHERE id = ?`, status, now, id)
	if err != nil {
		return fmt.Errorf("update follow-up status: %w", err)
	}
	return requireRow(res, "follow-up", id)
}

func (r *FollowUpRepo) statusOf(id string) (string, error) {
	var status string
	err := r.db.QueryRow(`SELECT status FROM follow_ups WHERE id = ?`, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", models.NewError(models.ErrNotFound, "follow-up %s not found", id)
	}
	if err != nil {
		return "", fmt.Errorf("read follow-up status: %w", err)
	}
	return status, nil
}
