package repository

import (
	"database/sql"
	"fmt"
	"relationship/internal/models"
)

type TraitRepo struct {
	db *sql.DB
}

func NewTraitRepo(db *sql.DB) *TraitRepo {
	return &TraitRepo{db: db}
}

func (r *TraitRepo) Create(trait *models.Trait) error {
	query := `
		INSERT INTO traits (id, person_id, trait_key, trait_value, confidence, source_event_ids, verified)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, trait.ID, trait.PersonID, trait.TraitKey, trait.TraitValue,
		trait.Confidence, trait.SourceEventIDs, trait.Verified)
	if err != nil {
		return fmt.Errorf("insert trait: %w", err)
	}
	return nil
}

func (r *TraitRepo) GetByID(id string) (*models.Trait, error) {
	query := `
		SELECT id, person_id, trait_key, trait_value, confidence, source_event_ids, verified, updated_at
		FROM traits WHERE id = ?
	`
	trait := &models.Trait{}
	err := r.db.QueryRow(query, id).Scan(
		&trait.ID, &trait.PersonID, &trait.TraitKey, &trait.TraitValue,
		&trait.Confidence, &trait.SourceEventIDs, &trait.Verified, &trait.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get trait: %w", err)
	}
	return trait, nil
}

func (r *TraitRepo) ListByPerson(personID string) ([]*models.Trait, error) {
	query := `
		SELECT id, person_id, trait_key, trait_value, confidence, source_event_ids, verified, updated_at
		FROM traits WHERE person_id = ? AND verified != -1
		ORDER BY confidence DESC
	`
	rows, err := r.db.Query(query, personID)
	if err != nil {
		return nil, fmt.Errorf("list traits: %w", err)
	}
	defer rows.Close()

	var traits []*models.Trait
	for rows.Next() {
		trait := &models.Trait{}
		if err := rows.Scan(
			&trait.ID, &trait.PersonID, &trait.TraitKey, &trait.TraitValue,
			&trait.Confidence, &trait.SourceEventIDs, &trait.Verified, &trait.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan trait: %w", err)
		}
		traits = append(traits, trait)
	}
	return traits, nil
}

func (r *TraitRepo) UpdateVerified(id string, verified int) error {
	query := `
		UPDATE traits SET verified = ?, updated_at = datetime('now')
		WHERE id = ?
	`
	_, err := r.db.Exec(query, verified, id)
	if err != nil {
		return fmt.Errorf("update trait verified: %w", err)
	}
	return nil
}

func (r *TraitRepo) Upsert(trait *models.Trait) error {
	query := `
		INSERT INTO traits (id, person_id, trait_key, trait_value, confidence, source_event_ids, verified)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(person_id, trait_key) DO UPDATE SET
			trait_value = excluded.trait_value,
			confidence = excluded.confidence,
			source_event_ids = excluded.source_event_ids,
			updated_at = datetime('now')
	`
	_, err := r.db.Exec(query, trait.ID, trait.PersonID, trait.TraitKey, trait.TraitValue,
		trait.Confidence, trait.SourceEventIDs, trait.Verified)
	if err != nil {
		return fmt.Errorf("upsert trait: %w", err)
	}
	return nil
}