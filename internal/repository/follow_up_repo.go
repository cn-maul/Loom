package repository

import (
	"database/sql"
	"fmt"
	"relationship/internal/models"
)

type FollowUpRepo struct{ db *sql.DB }

func NewFollowUpRepo(db *sql.DB) *FollowUpRepo { return &FollowUpRepo{db: db} }

const followUpColumns = `id, person_id, title, description, due_date, status, created_at, updated_at, completed_at`

func (r *FollowUpRepo) Create(f *models.FollowUp) error {
	now, t := models.NowUTC()
	_, err := r.db.Exec(`INSERT INTO follow_ups (id, person_id, title, description, due_date, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, f.ID, f.PersonID, f.Title, f.Description, f.DueDate, f.Status, now, now)
	if err != nil {
		return fmt.Errorf("create follow-up: %w", err)
	}
	f.CreatedAt, f.UpdatedAt = t, t
	return nil
}

func (r *FollowUpRepo) Get(id string) (*models.FollowUp, error) {
	f := &models.FollowUp{}
	var created, updated, completed string
	err := r.db.QueryRow(`SELECT `+followUpColumns+` FROM follow_ups WHERE id = ?`, id).Scan(&f.ID, &f.PersonID, &f.Title, &f.Description, &f.DueDate, &f.Status, &created, &updated, &completed)
	if err != nil {
		return nil, fmt.Errorf("get follow-up: %w", err)
	}
	f.CreatedAt, f.UpdatedAt, f.CompletedAt = models.ParseSQLiteTime(created), models.ParseSQLiteTime(updated), models.ParseSQLiteTime(completed)
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
	query += ` ORDER BY CASE WHEN due_date = '' THEN 1 ELSE 0 END, due_date, created_at`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list follow-ups: %w", err)
	}
	defer rows.Close()
	var result []*models.FollowUp
	for rows.Next() {
		f := &models.FollowUp{}
		var created, updated, completed string
		if err := rows.Scan(&f.ID, &f.PersonID, &f.Title, &f.Description, &f.DueDate, &f.Status, &created, &updated, &completed); err != nil {
			return nil, fmt.Errorf("scan follow-up: %w", err)
		}
		f.CreatedAt, f.UpdatedAt, f.CompletedAt = models.ParseSQLiteTime(created), models.ParseSQLiteTime(updated), models.ParseSQLiteTime(completed)
		result = append(result, f)
	}
	return result, rows.Err()
}

func (r *FollowUpRepo) Update(f *models.FollowUp) error {
	_, err := r.db.Exec(`UPDATE follow_ups SET title = ?, description = ?, due_date = ?, status = ?, updated_at = datetime('now') WHERE id = ?`, f.Title, f.Description, f.DueDate, f.Status, f.ID)
	if err != nil {
		return fmt.Errorf("update follow-up: %w", err)
	}
	return nil
}

func (r *FollowUpRepo) Postpone(id, dueDate string) error {
	_, err := r.db.Exec(`UPDATE follow_ups SET due_date = ?, status = 'waiting', completed_at = NULL, updated_at = datetime('now') WHERE id = ?`, dueDate, id)
	if err != nil {
		return fmt.Errorf("postpone follow-up: %w", err)
	}
	return nil
}

func (r *FollowUpRepo) Action(id, status string) error {
	_, err := r.db.Exec(`UPDATE follow_ups SET status = ?, updated_at = datetime('now'), completed_at = CASE WHEN ? = 'completed' THEN datetime('now') ELSE NULL END WHERE id = ?`, status, status, id)
	if err != nil {
		return fmt.Errorf("update follow-up status: %w", err)
	}
	return nil
}
