package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"relationship/internal/models"
)

type OrganizationRepo struct {
	db *sql.DB
}

func NewOrganizationRepo(db *sql.DB) *OrganizationRepo {
	return &OrganizationRepo{db: db}
}

const organizationColumns = `id, name, kind, description, archived_at, created_at, updated_at`

func scanOrganization(row rowScanner) (*models.Organization, error) {
	org := &models.Organization{}
	var kind, description, archived, created, updated sql.NullString
	if err := row.Scan(
		&org.ID, &org.Name, &kind, &description, &archived, &created, &updated,
	); err != nil {
		return nil, err
	}
	org.Kind = scanNullString(&kind)
	org.Description = scanNullString(&description)
	if archived.Valid && archived.String != "" {
		at := models.ParseSQLiteTime(archived.String)
		if !at.IsZero() {
			org.ArchivedAt = &at
		}
	}
	org.CreatedAt = models.ParseSQLiteTime(created.String)
	org.UpdatedAt = models.ParseSQLiteTime(updated.String)
	return org, nil
}

func (r *OrganizationRepo) Create(org *models.Organization) error {
	created, now := models.NowUTC()
	query := `
		INSERT INTO organizations (id, name, kind, description, archived_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULL, ?, ?)
	`
	if _, err := r.db.Exec(query, org.ID, org.Name, nullIfEmpty(org.Kind), nullIfEmpty(org.Description), created, created); err != nil {
		return fmt.Errorf("insert organization: %w", err)
	}
	org.CreatedAt = now
	org.UpdatedAt = now
	return nil
}

func (r *OrganizationRepo) GetByID(id string) (*models.Organization, error) {
	org, err := scanOrganization(r.db.QueryRow(`SELECT `+organizationColumns+` FROM organizations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.NewError(models.ErrNotFound, "organization %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return org, nil
}

// List returns active organisations only, in name order. Pickers (person form,
// home filter) use this; an archived employer must not be assignable again.
func (r *OrganizationRepo) List() ([]*models.Organization, error) {
	rows, err := r.db.Query(`SELECT ` + organizationColumns + ` FROM organizations
		WHERE archived_at IS NULL ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()

	orgs := []*models.Organization{}
	for rows.Next() {
		org, err := scanOrganization(rows)
		if err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

// ListAll returns every organisation, active first so the management page opens
// on what is still in use; archived ones follow at the bottom.
func (r *OrganizationRepo) ListAll() ([]*models.Organization, error) {
	rows, err := r.db.Query(`SELECT ` + organizationColumns + ` FROM organizations
		ORDER BY archived_at IS NOT NULL, name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list all organizations: %w", err)
	}
	defer rows.Close()

	orgs := []*models.Organization{}
	for rows.Next() {
		org, err := scanOrganization(rows)
		if err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

func (r *OrganizationRepo) Update(org *models.Organization) error {
	updated, now := models.NowUTC()
	query := `UPDATE organizations SET name = ?, kind = ?, description = ?, updated_at = ? WHERE id = ?`
	res, err := r.db.Exec(query, org.Name, nullIfEmpty(org.Kind), nullIfEmpty(org.Description), updated, org.ID)
	if err != nil {
		return fmt.Errorf("update organization: %w", err)
	}
	if err := requireRow(res, "organization", org.ID); err != nil {
		return err
	}
	org.UpdatedAt = now
	return nil
}

// Archive is a soft delete: the row stays so the people who were attached to it
// keep their employer, but it drops out of every picker. Restore clears the
// stamp and the organisation is assignable again.
func (r *OrganizationRepo) Archive(id string) error {
	archived, _ := models.NowUTC()
	res, err := r.db.Exec(`UPDATE organizations SET archived_at = ?, updated_at = ? WHERE id = ?`, archived, archived, id)
	if err != nil {
		return fmt.Errorf("archive organization: %w", err)
	}
	return requireRow(res, "organization", id)
}

func (r *OrganizationRepo) Restore(id string) error {
	updated, _ := models.NowUTC()
	res, err := r.db.Exec(`UPDATE organizations SET archived_at = NULL, updated_at = ? WHERE id = ?`, updated, id)
	if err != nil {
		return fmt.Errorf("restore organization: %w", err)
	}
	return requireRow(res, "organization", id)
}

// Delete drops the organization and detaches its members. The foreign key
// already handles the detachment; the explicit UPDATE is a belt-and-braces
// guard for connections opened without foreign_keys=ON.
func (r *OrganizationRepo) Delete(id string) error {
	if _, err := r.db.Exec(`UPDATE persons SET org_id = NULL WHERE org_id = ?`, id); err != nil {
		return fmt.Errorf("detach organization members: %w", err)
	}
	res, err := r.db.Exec(`DELETE FROM organizations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete organization: %w", err)
	}
	return requireRow(res, "organization", id)
}
