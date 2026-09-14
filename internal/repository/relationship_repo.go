package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"relationship/internal/models"
)

type RelationshipRepo struct{ db *sql.DB }

func NewRelationshipRepo(db *sql.DB) *RelationshipRepo { return &RelationshipRepo{db: db} }

// relationshipSelect always resolves both endpoint names, so a list of edges does
// not need a lookup per row.
const relationshipSelect = `SELECT r.id, r.from_person_id, r.to_person_id, r.relation_type, r.direction,
		r.start_date, r.end_date, r.source_event_id, r.confirmed, r.notes, r.created_at, r.updated_at,
		COALESCE(fp.name, ''), COALESCE(tp.name, '')
	FROM person_relationships r
	LEFT JOIN persons fp ON fp.id = r.from_person_id
	LEFT JOIN persons tp ON tp.id = r.to_person_id`

func scanRelationship(row rowScanner) (*models.RelationshipLink, error) {
	link := &models.RelationshipLink{}
	var startDate, endDate, sourceEventID, notes, created, updated sql.NullString
	if err := row.Scan(
		&link.ID, &link.FromPersonID, &link.ToPersonID, &link.RelationType, &link.Direction,
		&startDate, &endDate, &sourceEventID, &link.Confirmed, &notes, &created, &updated,
		&link.FromPersonName, &link.ToPersonName,
	); err != nil {
		return nil, err
	}
	link.StartDate = scanNullString(&startDate)
	link.EndDate = scanNullString(&endDate)
	link.SourceEventID = scanNullString(&sourceEventID)
	link.Notes = scanNullString(&notes)
	link.CreatedAt = models.ParseSQLiteTime(scanNullString(&created))
	link.UpdatedAt = models.ParseSQLiteTime(scanNullString(&updated))
	return link, nil
}

func (r *RelationshipRepo) Create(rel *models.Relationship) error {
	created, now := models.NowUTC()
	_, err := r.db.Exec(`
		INSERT INTO person_relationships (id, from_person_id, to_person_id, relation_type, direction,
			start_date, end_date, source_event_id, confirmed, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rel.ID, rel.FromPersonID, rel.ToPersonID, rel.RelationType, rel.Direction,
		nullIfEmpty(rel.StartDate), nullIfEmpty(rel.EndDate), nullIfEmpty(rel.SourceEventID),
		rel.Confirmed, nullIfEmpty(rel.Notes), created, created,
	)
	if err != nil {
		return fmt.Errorf("insert relationship: %w", err)
	}
	rel.CreatedAt, rel.UpdatedAt = now, now
	return nil
}

func (r *RelationshipRepo) GetByID(id string) (*models.RelationshipLink, error) {
	link, err := scanRelationship(r.db.QueryRow(relationshipSelect+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.NewError(models.ErrNotFound, "relationship %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get relationship: %w", err)
	}
	return link, nil
}

// List returns the graph edges matching the filter. PersonID matches either end,
// so a person's page shows both the edges they own and the ones pointing at them.
func (r *RelationshipRepo) List(filter models.RelationshipFilter) ([]*models.RelationshipLink, error) {
	query := relationshipSelect + ` WHERE 1 = 1`
	args := []any{}
	if filter.PersonID != "" {
		query += ` AND (r.from_person_id = ? OR r.to_person_id = ?)`
		args = append(args, filter.PersonID, filter.PersonID)
	}
	if filter.Type != "" {
		query += ` AND r.relation_type = ?`
		args = append(args, filter.Type)
	}
	if filter.Confirmed != nil {
		query += ` AND r.confirmed = ?`
		args = append(args, *filter.Confirmed)
	}
	if filter.Active != nil {
		if *filter.Active {
			query += ` AND r.end_date IS NULL`
		} else {
			query += ` AND r.end_date IS NOT NULL`
		}
	}
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		query += ` AND (r.relation_type LIKE ? OR COALESCE(r.notes, '') LIKE ?)`
		args = append(args, like, like)
	}
	// Open edges first, then the most recent start date; undated edges last.
	query += ` ORDER BY CASE WHEN r.end_date IS NULL THEN 0 ELSE 1 END,
		COALESCE(r.start_date, '') DESC, r.created_at DESC`
	if filter.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, filter.Limit, filter.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list relationships: %w", err)
	}
	defer rows.Close()

	links := []*models.RelationshipLink{}
	for rows.Next() {
		link, err := scanRelationship(rows)
		if err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// Update writes the mutable fields. An end_date is how a relationship is closed
// without deleting it, which is what keeps the history readable.
func (r *RelationshipRepo) Update(rel *models.Relationship) error {
	updated, _ := models.NowUTC()
	res, err := r.db.Exec(`
		UPDATE person_relationships SET from_person_id = ?, to_person_id = ?, relation_type = ?, direction = ?,
			start_date = ?, end_date = ?, source_event_id = ?, confirmed = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		rel.FromPersonID, rel.ToPersonID, rel.RelationType, rel.Direction,
		nullIfEmpty(rel.StartDate), nullIfEmpty(rel.EndDate), nullIfEmpty(rel.SourceEventID),
		rel.Confirmed, nullIfEmpty(rel.Notes), updated, rel.ID,
	)
	if err != nil {
		return fmt.Errorf("update relationship: %w", err)
	}
	return requireRow(res, "relationship", rel.ID)
}

func (r *RelationshipRepo) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM person_relationships WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete relationship: %w", err)
	}
	return requireRow(res, "relationship", id)
}

// ListTypes returns the distinct relation types already in use, so a picker can
// offer them instead of forcing free text.
func (r *RelationshipRepo) ListTypes() ([]string, error) {
	rows, err := r.db.Query(`SELECT DISTINCT relation_type FROM person_relationships ORDER BY relation_type`)
	if err != nil {
		return nil, fmt.Errorf("list relationship types: %w", err)
	}
	defer rows.Close()
	types := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan relationship type: %w", err)
		}
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			types = append(types, trimmed)
		}
	}
	return types, rows.Err()
}
