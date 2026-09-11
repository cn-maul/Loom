package repository

import (
	"database/sql"
	"fmt"
	"relationship/internal/models"
)

type PersonRepo struct {
	db *sql.DB
}

func NewPersonRepo(db *sql.DB) *PersonRepo {
	return &PersonRepo{db: db}
}

const personColumns = `id, name, relation, importance, notes, org_id, position, created_at, updated_at`

// nullIfEmpty maps "" to NULL so an emptied affiliation clears the column
// instead of storing an empty string.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *PersonRepo) Create(person *models.Person) error {
	created, now := models.NowUTC()
	query := `
		INSERT INTO persons (id, name, relation, importance, notes, org_id, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, person.ID, person.Name, person.Relation, person.Importance, person.Notes,
		nullIfEmpty(person.OrgID), nullIfEmpty(person.Position), created, created)
	if err != nil {
		return fmt.Errorf("insert person: %w", err)
	}
	person.CreatedAt = now
	person.UpdatedAt = now
	return nil
}

func (r *PersonRepo) GetByID(id string) (*models.Person, error) {
	person := &models.Person{}
	var created, updated string
	var orgID, position sql.NullString
	err := r.db.QueryRow(`SELECT `+personColumns+` FROM persons WHERE id = ?`, id).Scan(
		&person.ID, &person.Name, &person.Relation, &person.Importance,
		&person.Notes, &orgID, &position, &created, &updated,
	)
	if err != nil {
		return nil, fmt.Errorf("get person: %w", err)
	}
	person.OrgID = scanNullString(&orgID)
	person.Position = scanNullString(&position)
	person.CreatedAt = models.ParseSQLiteTime(created)
	person.UpdatedAt = models.ParseSQLiteTime(updated)
	return person, nil
}

// ListWithActivity returns every person with the timeline metadata the home page
// needs, ordered by most recent interaction rather than by record edits.
func (r *PersonRepo) ListWithActivity() ([]*models.PersonWithActivity, error) {
	rows, err := r.db.Query(`
		SELECT p.id, p.name, p.relation, p.importance, p.notes, p.org_id, p.position, p.created_at, p.updated_at,
		       o.name AS org_name,
		       MAX(e.event_date) AS last_event_date, COUNT(e.id) AS event_count
		FROM persons p
		LEFT JOIN organizations o ON o.id = p.org_id
		LEFT JOIN events e ON e.person_id = p.id
		GROUP BY p.id
		ORDER BY COALESCE(last_event_date, p.created_at) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list persons: %w", err)
	}
	defer rows.Close()

	var persons []*models.PersonWithActivity
	for rows.Next() {
		person := &models.PersonWithActivity{}
		var created, updated string
		var orgID, position, lastEvent, orgName sql.NullString
		if err := rows.Scan(
			&person.ID, &person.Name, &person.Relation, &person.Importance,
			&person.Notes, &orgID, &position, &created, &updated,
			&orgName, &lastEvent, &person.EventCount,
		); err != nil {
			return nil, fmt.Errorf("scan person: %w", err)
		}
		person.OrgID = scanNullString(&orgID)
		person.Position = scanNullString(&position)
		person.CreatedAt = models.ParseSQLiteTime(created)
		person.UpdatedAt = models.ParseSQLiteTime(updated)
		person.LastEventDate = scanNullString(&lastEvent)
		person.OrgName = scanNullString(&orgName)
		persons = append(persons, person)
	}
	return persons, rows.Err()
}

func (r *PersonRepo) Update(person *models.Person) error {
	updated, now := models.NowUTC()
	query := `
		UPDATE persons SET name = ?, relation = ?, importance = ?, notes = ?, org_id = ?, position = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := r.db.Exec(query, person.Name, person.Relation, person.Importance, person.Notes,
		nullIfEmpty(person.OrgID), nullIfEmpty(person.Position), updated, person.ID)
	if err != nil {
		return fmt.Errorf("update person: %w", err)
	}
	person.UpdatedAt = now
	return nil
}

func (r *PersonRepo) Delete(id string) error {
	if _, err := r.db.Exec(`DELETE FROM persons WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete person: %w", err)
	}
	return nil
}
