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

func (r *PersonRepo) Create(person *models.Person) error {
	query := `
		INSERT INTO persons (id, name, relation, importance, notes)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, person.ID, person.Name, person.Relation, person.Importance, person.Notes)
	if err != nil {
		return fmt.Errorf("insert person: %w", err)
	}
	return nil
}

func (r *PersonRepo) GetByID(id string) (*models.Person, error) {
	query := `
		SELECT id, name, relation, importance, notes, created_at, updated_at
		FROM persons WHERE id = ?
	`
	person := &models.Person{}
	err := r.db.QueryRow(query, id).Scan(
		&person.ID, &person.Name, &person.Relation, &person.Importance,
		&person.Notes, &person.CreatedAt, &person.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get person: %w", err)
	}
	return person, nil
}

func (r *PersonRepo) List() ([]*models.Person, error) {
	query := `
		SELECT id, name, relation, importance, notes, created_at, updated_at
		FROM persons ORDER BY updated_at DESC
	`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list persons: %w", err)
	}
	defer rows.Close()

	var persons []*models.Person
	for rows.Next() {
		person := &models.Person{}
		if err := rows.Scan(
			&person.ID, &person.Name, &person.Relation, &person.Importance,
			&person.Notes, &person.CreatedAt, &person.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan person: %w", err)
		}
		persons = append(persons, person)
	}
	return persons, nil
}

func (r *PersonRepo) Update(person *models.Person) error {
	query := `
		UPDATE persons SET name = ?, relation = ?, importance = ?, notes = ?, updated_at = datetime('now')
		WHERE id = ?
	`
	_, err := r.db.Exec(query, person.Name, person.Relation, person.Importance, person.Notes, person.ID)
	if err != nil {
		return fmt.Errorf("update person: %w", err)
	}
	return nil
}

func (r *PersonRepo) Delete(id string) error {
	query := `DELETE FROM persons WHERE id = ?`
	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("delete person: %w", err)
	}
	return nil
}