package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"relationship/internal/models"
)

type TraitRepo struct {
	db *sql.DB
}

func NewTraitRepo(db *sql.DB) *TraitRepo {
	return &TraitRepo{db: db}
}

const traitColumns = `id, person_id, trait_key, trait_value, confidence, source_event_ids, verified, updated_at, source_stale, source_stale_reason`

// Upsert keeps the row identity of an existing trait, so its vector id and the
// user's verification verdict survive a profile refresh. The staleness marker is
// cleared as well: the row has just been re-derived from the records, so it is
// no longer describing a source that changed.
func (r *TraitRepo) Upsert(trait *models.Trait) error {
	query := `
		INSERT INTO traits (id, person_id, trait_key, trait_value, confidence, source_event_ids, verified, source_stale)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(person_id, trait_key) DO UPDATE SET
			trait_value = excluded.trait_value,
			confidence = excluded.confidence,
			source_event_ids = excluded.source_event_ids,
			source_stale = 0,
			source_stale_reason = NULL,
			updated_at = datetime('now')
	`
	_, err := r.db.Exec(query, trait.ID, trait.PersonID, trait.TraitKey, trait.TraitValue,
		trait.Confidence, trait.SourceEventIDs, trait.Verified)
	if err != nil {
		return fmt.Errorf("upsert trait: %w", err)
	}
	return nil
}

// MarkSourceStale flags every profile note that cites an edited or deleted
// record. The row is kept and annotated rather than dropped: silently removing
// it would hide the fact that a conclusion the user may be relying on has lost
// its evidence. Matching is a LIKE on the stored JSON id list, which is exact
// because the ids are UUIDs and cannot be substrings of one another.
func (r *TraitRepo) MarkSourceStale(eventID, reason string) (int, error) {
	res, err := r.db.Exec(`
		UPDATE traits SET source_stale = 1, source_stale_reason = ?
		WHERE source_stale = 0 AND source_event_ids LIKE ?`,
		reason, "%\""+strings.TrimSpace(eventID)+"\"%")
	if err != nil {
		return 0, fmt.Errorf("mark traits stale: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read affected traits: %w", err)
	}
	return int(affected), nil
}

func (r *TraitRepo) GetByID(id string) (*models.Trait, error) {
	trait, err := scanTrait(r.db.QueryRow(`SELECT `+traitColumns+` FROM traits WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("get trait: %w", err)
	}
	return trait, nil
}

// GetByKey reads back the stored row; its id is what an upsert preserves.
func (r *TraitRepo) GetByKey(personID, traitKey string) (*models.Trait, error) {
	trait, err := scanTrait(r.db.QueryRow(
		`SELECT `+traitColumns+` FROM traits WHERE person_id = ? AND trait_key = ?`, personID, traitKey))
	if err != nil {
		return nil, fmt.Errorf("get trait by key: %w", err)
	}
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

// ListByPerson hides traits the user rejected. Stale ones are still returned,
// carrying their marker, because "this conclusion lost its evidence" is
// information the person page has to be able to show.
func (r *TraitRepo) ListByPerson(personID string) ([]*models.Trait, error) {
	rows, err := r.db.Query(`SELECT `+traitColumns+` FROM traits
		WHERE person_id = ? AND verified != -1
		ORDER BY source_stale, confidence DESC`, personID)
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

func scanTrait(row rowScanner) (*models.Trait, error) {
	trait := &models.Trait{}
	var updated, staleReason sql.NullString
	if err := row.Scan(
		&trait.ID, &trait.PersonID, &trait.TraitKey, &trait.TraitValue,
		&trait.Confidence, &trait.SourceEventIDs, &trait.Verified, &updated,
		&trait.SourceStale, &staleReason,
	); err != nil {
		return nil, err
	}
	trait.UpdatedAt = models.ParseSQLiteTime(scanNullString(&updated))
	trait.SourceStaleReason = scanNullString(&staleReason)
	return trait, nil
}

func scanTraits(rows *sql.Rows) ([]*models.Trait, error) {
	var traits []*models.Trait
	for rows.Next() {
		trait, err := scanTrait(rows)
		if err != nil {
			return nil, fmt.Errorf("scan trait: %w", err)
		}
		traits = append(traits, trait)
	}
	return traits, rows.Err()
}
