package repository

import (
	"database/sql"
	"fmt"
	"relationship/internal/models"
	"strings"
)

type EventRepo struct {
	db *sql.DB
}

func NewEventRepo(db *sql.DB) *EventRepo {
	return &EventRepo{db: db}
}

const eventColumns = `id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises, created_at`

func (r *EventRepo) Create(event *models.Event) error {
	created, now := models.NowUTC()
	query := `
		INSERT INTO events (id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, event.ID, event.PersonID, event.RawText, event.EventDate,
		event.Summary, event.MyFeeling, event.TheirReaction, event.Promises, created)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	event.CreatedAt = now
	return nil
}

func (r *EventRepo) GetByID(id string) (*models.Event, error) {
	event := &models.Event{}
	var created string
	err := r.db.QueryRow(`SELECT `+eventColumns+` FROM events WHERE id = ?`, id).Scan(
		&event.ID, &event.PersonID, &event.RawText, &event.EventDate,
		&event.Summary, &event.MyFeeling, &event.TheirReaction, &event.Promises, &created,
	)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	event.CreatedAt = models.ParseSQLiteTime(created)
	return event, nil
}

func (r *EventRepo) ListByPerson(personID string, limit int) ([]*models.Event, error) {
	rows, err := r.db.Query(`SELECT `+eventColumns+` FROM events WHERE person_id = ?
		ORDER BY event_date DESC LIMIT ?`, personID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// ListSince returns events from a given YYYY-MM-DD date, optionally for one person.
func (r *EventRepo) ListSince(since string, personID string) ([]*models.Event, error) {
	query := `SELECT ` + eventColumns + ` FROM events WHERE event_date >= ?`
	args := []interface{}{since}
	if personID != "" {
		query += ` AND person_id = ?`
		args = append(args, personID)
	}
	query += ` ORDER BY event_date ASC, created_at ASC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events since %s: %w", since, err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (r *EventRepo) ListAll() ([]*models.Event, error) {
	rows, err := r.db.Query(`SELECT ` + eventColumns + ` FROM events ORDER BY event_date ASC`)
	if err != nil {
		return nil, fmt.Errorf("list all events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (r *EventRepo) Delete(id string) error {
	if _, err := r.db.Exec(`DELETE FROM events WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return nil
}

func (r *EventRepo) Update(event *models.Event) error {
	query := `
		UPDATE events SET summary = ?, my_feeling = ?, their_reaction = ?, promises = ?
		WHERE id = ?
	`
	if _, err := r.db.Exec(query, event.Summary, event.MyFeeling, event.TheirReaction, event.Promises, event.ID); err != nil {
		return fmt.Errorf("update event: %w", err)
	}
	return nil
}

func scanEvents(rows *sql.Rows) ([]*models.Event, error) {
	var events []*models.Event
	for rows.Next() {
		event := &models.Event{}
		var created string
		if err := rows.Scan(
			&event.ID, &event.PersonID, &event.RawText, &event.EventDate,
			&event.Summary, &event.MyFeeling, &event.TheirReaction, &event.Promises, &created,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.CreatedAt = models.ParseSQLiteTime(created)
		events = append(events, event)
	}
	return events, rows.Err()
}

// scanNullableTime is used by repositories reading columns the schema may leave null.
func scanNullString(s *sql.NullString) string {
	if !s.Valid {
		return ""
	}
	return strings.TrimSpace(s.String)
}
