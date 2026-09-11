package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"relationship/internal/models"
)

type PositionRepo struct{ db *sql.DB }

func NewPositionRepo(db *sql.DB) *PositionRepo { return &PositionRepo{db: db} }

const positionSelect = `SELECT pos.id, pos.person_id, pos.org_id, pos.role, pos.start_date, pos.end_date,
		pos.source, pos.notes, pos.created_at, pos.updated_at,
		COALESCE(p.name, ''), COALESCE(o.name, '')
	FROM person_org_positions pos
	LEFT JOIN persons p ON p.id = pos.person_id
	LEFT JOIN organizations o ON o.id = pos.org_id`

func scanPosition(row rowScanner) (*models.OrgPositionLink, error) {
	link := &models.OrgPositionLink{}
	var role, startDate, endDate, source, notes, created, updated sql.NullString
	if err := row.Scan(
		&link.ID, &link.PersonID, &link.OrgID, &role, &startDate, &endDate,
		&source, &notes, &created, &updated, &link.PersonName, &link.OrgName,
	); err != nil {
		return nil, err
	}
	link.Role = scanNullString(&role)
	link.StartDate = scanNullString(&startDate)
	link.EndDate = scanNullString(&endDate)
	link.Source = scanNullString(&source)
	link.Notes = scanNullString(&notes)
	link.CreatedAt = models.ParseSQLiteTime(scanNullString(&created))
	link.UpdatedAt = models.ParseSQLiteTime(scanNullString(&updated))
	return link, nil
}

func (r *PositionRepo) Create(pos *models.OrgPosition) error {
	created, now := models.NowUTC()
	_, err := r.db.Exec(`
		INSERT INTO person_org_positions (id, person_id, org_id, role, start_date, end_date, source, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pos.ID, pos.PersonID, pos.OrgID, nullIfEmpty(pos.Role),
		nullIfEmpty(pos.StartDate), nullIfEmpty(pos.EndDate),
		nullIfEmpty(pos.Source), nullIfEmpty(pos.Notes), created, created,
	)
	if err != nil {
		return fmt.Errorf("insert position: %w", err)
	}
	pos.CreatedAt, pos.UpdatedAt = now, now
	return nil
}

func (r *PositionRepo) GetByID(id string) (*models.OrgPositionLink, error) {
	link, err := scanPosition(r.db.QueryRow(positionSelect+` WHERE pos.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.NewError(models.ErrNotFound, "position %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get position: %w", err)
	}
	return link, nil
}

// ListByPerson returns the full stint history for one person, current postings
// first. Ended postings stay in the list, which is what makes the history
// readable after someone leaves an organisation.
func (r *PositionRepo) ListByPerson(personID string) ([]*models.OrgPositionLink, error) {
	return r.query(positionSelect+` WHERE pos.person_id = ?
		ORDER BY CASE WHEN pos.end_date IS NULL THEN 0 ELSE 1 END, COALESCE(pos.start_date, '') DESC`, personID)
}

// ListAll returns every posting with both names resolved. A period report covers
// whoever changed, which is not knowable before reading the rows.
func (r *PositionRepo) ListAll() ([]*models.OrgPositionLink, error) {
	return r.query(positionSelect + ` ORDER BY COALESCE(pos.start_date, '') DESC, pos.created_at DESC`)
}

// ListByOrg returns an organisation's members. currentOnly keeps just the people
// whose end_date is still NULL.
func (r *PositionRepo) ListByOrg(orgID string, currentOnly bool) ([]*models.OrgPositionLink, error) {
	query := positionSelect + ` WHERE pos.org_id = ?`
	if currentOnly {
		query += ` AND pos.end_date IS NULL`
	}
	query += ` ORDER BY CASE WHEN pos.end_date IS NULL THEN 0 ELSE 1 END, COALESCE(pos.start_date, '') DESC`
	return r.query(query, orgID)
}

func (r *PositionRepo) Update(pos *models.OrgPosition) error {
	updated, _ := models.NowUTC()
	res, err := r.db.Exec(`
		UPDATE person_org_positions SET person_id = ?, org_id = ?, role = ?, start_date = ?, end_date = ?,
			source = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		pos.PersonID, pos.OrgID, nullIfEmpty(pos.Role),
		nullIfEmpty(pos.StartDate), nullIfEmpty(pos.EndDate),
		nullIfEmpty(pos.Source), nullIfEmpty(pos.Notes), updated, pos.ID,
	)
	if err != nil {
		return fmt.Errorf("update position: %w", err)
	}
	return requireRow(res, "position", pos.ID)
}

func (r *PositionRepo) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM person_org_positions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete position: %w", err)
	}
	return requireRow(res, "position", id)
}

func (r *PositionRepo) query(query string, args ...any) ([]*models.OrgPositionLink, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	defer rows.Close()

	links := []*models.OrgPositionLink{}
	for rows.Next() {
		link, err := scanPosition(rows)
		if err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}
