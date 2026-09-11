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

const traitColumns = `id, person_id, trait_key, trait_value, confidence, source_event_ids, verified, updated_at`

// Upsert keeps the row identity of an existing trait, so its vector id and the
// user's verification verdict survive a profile refresh.
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

func (r *TraitRepo) GetByID(id string) (*models.Trait, error) {
	trait := &models.Trait{}
	var updated string
	err := r.db.QueryRow(`SELECT `+traitColumns+` FROM traits WHERE id = ?`, id).Scan(
		&trait.ID, &trait.PersonID, &trait.TraitKey, &trait.TraitValue,
		&trait.Confidence, &trait.SourceEventIDs, &trait.Verified, &updated,
	)
	if err != nil {
		return nil, fmt.Errorf("get trait: %w", err)
	}
	trait.UpdatedAt = models.ParseSQLiteTime(updated)
	return trait, nil
}

// GetByKey reads back the stored row; its id is what an upsert preserves.
func (r *TraitRepo) GetByKey(personID, traitKey string) (*models.Trait, error) {
	trait := &models.Trait{}
	var updated string
	err := r.db.QueryRow(`SELECT `+traitColumns+` FROM traits WHERE person_id = ? AND trait_key = ?`,
		personID, traitKey).Scan(
		&trait.ID, &trait.PersonID, &trait.TraitKey, &trait.TraitValue,
		&trait.Confidence, &trait.SourceEventIDs, &trait.Verified, &updated,
	)
	if err != nil {
		return nil, fmt.Errorf("get trait by key: %w", err)
	}
	trait.UpdatedAt = models.ParseSQLiteTime(updated)
	return trait, nil
}

// ListRejectedKeys returns the dimensions the user has rejected, so a profile
// refresh can be told not to re-propose them.
func (r *TraitRepo) ListRejectedKeys(personID string) ([]string, error) {
	rows, err := r.db.Query(`SELECT trait_key FROM traits WHERE person_id = ? AND verified = -1`, personID)
	if err != nil {
		return nil, fmt.Errorf("list rejected traits: %w", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan rejected trait: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// ListByPerson hides traits the user rejected.
func (r *TraitRepo) ListByPerson(personID string) ([]*models.Trait, error) {
	rows, err := r.db.Query(`SELECT `+traitColumns+` FROM traits
		WHERE person_id = ? AND verified != -1
		ORDER BY confidence DESC`, personID)
	if err != nil {
		return nil, fmt.Errorf("list traits: %w", err)
	}
	defer rows.Close()
	return scanTraits(rows)
}

func (r *TraitRepo) ListAll() ([]*models.Trait, error) {
	rows, err := r.db.Query(`SELECT ` + traitColumns + ` FROM traits WHERE verified != -1`)
	if err != nil {
		return nil, fmt.Errorf("list all traits: %w", err)
	}
	defer rows.Close()
	return scanTraits(rows)
}

func (r *TraitRepo) UpdateVerified(id string, verified int) error {
	query := `
		UPDATE traits SET verified = ?, updated_at = datetime('now')
		WHERE id = ?
	`
	if _, err := r.db.Exec(query, verified, id); err != nil {
		return fmt.Errorf("update trait verified: %w", err)
	}
	return nil
}

func scanTraits(rows *sql.Rows) ([]*models.Trait, error) {
	var traits []*models.Trait
	for rows.Next() {
		trait := &models.Trait{}
		var updated string
		if err := rows.Scan(
			&trait.ID, &trait.PersonID, &trait.TraitKey, &trait.TraitValue,
			&trait.Confidence, &trait.SourceEventIDs, &trait.Verified, &updated,
		); err != nil {
			return nil, fmt.Errorf("scan trait: %w", err)
		}
		trait.UpdatedAt = models.ParseSQLiteTime(updated)
		traits = append(traits, trait)
	}
	return traits, rows.Err()
}
