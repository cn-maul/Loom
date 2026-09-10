package repository

import (
	"database/sql"
	"fmt"
	"relationship/internal/models"
)

type EventRepo struct {
	db *sql.DB
}

func NewEventRepo(db *sql.DB) *EventRepo {
	return &EventRepo{db: db}
}

func (r *EventRepo) Create(event *models.Event) error {
	query := `
		INSERT INTO events (id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, event.ID, event.PersonID, event.RawText, event.EventDate,
		event.Summary, event.MyFeeling, event.TheirReaction, event.Promises)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (r *EventRepo) GetByID(id string) (*models.Event, error) {
	query := `
		SELECT id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises, created_at
		FROM events WHERE id = ?
	`
	event := &models.Event{}
	err := r.db.QueryRow(query, id).Scan(
		&event.ID, &event.PersonID, &event.RawText, &event.EventDate,
		&event.Summary, &event.MyFeeling, &event.TheirReaction, &event.Promises, &event.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	return event, nil
}

func (r *EventRepo) ListByPerson(personID string, limit int) ([]*models.Event, error) {
	query := `
		SELECT id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises, created_at
		FROM events WHERE person_id = ?
		ORDER BY event_date DESC
		LIMIT ?
	`
	rows, err := r.db.Query(query, personID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []*models.Event
	for rows.Next() {
		event := &models.Event{}
		if err := rows.Scan(
			&event.ID, &event.PersonID, &event.RawText, &event.EventDate,
			&event.Summary, &event.MyFeeling, &event.TheirReaction, &event.Promises, &event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, event)
	}
	return events, nil
}

func (r *EventRepo) Delete(id string) error {
	query := `DELETE FROM events WHERE id = ?`
	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return nil
}

func (r *EventRepo) Update(event *models.Event) error {
	query := `
		UPDATE events SET summary = ?, my_feeling = ?, their_reaction = ?, promises = ?
		WHERE id = ?
	`
	_, err := r.db.Exec(query, event.Summary, event.MyFeeling, event.TheirReaction, event.Promises, event.ID)
	if err != nil {
		return fmt.Errorf("update event: %w", err)
	}
	return nil
}