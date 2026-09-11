package repository

import (
	"database/sql"
	"fmt"
	"relationship/internal/models"
)

type OrganizationRepo struct {
	db *sql.DB
}

func NewOrganizationRepo(db *sql.DB) *OrganizationRepo {
	return &OrganizationRepo{db: db}
}

const organizationColumns = `id, name, created_at, updated_at`

func (r *OrganizationRepo) Create(org *models.Organization) error {
	created, now := models.NowUTC()
	query := `
		INSERT INTO organizations (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`
	if _, err := r.db.Exec(query, org.ID, org.Name, created, created); err != nil {
		return fmt.Errorf("insert organization: %w", err)
	}
	org.CreatedAt = now
	org.UpdatedAt = now
	return nil
}

func (r *OrganizationRepo) GetByID(id string) (*models.Organization, error) {
	org := &models.Organization{}
	var created, updated string
	err := r.db.QueryRow(`SELECT `+organizationColumns+` FROM organizations WHERE id = ?`, id).Scan(
		&org.ID, &org.Name, &created, &updated,
	)
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	org.CreatedAt = models.ParseSQLiteTime(created)
	org.UpdatedAt = models.ParseSQLiteTime(updated)
	return org, nil
}

func (r *OrganizationRepo) List() ([]*models.Organization, error) {
	rows, err := r.db.Query(`SELECT ` + organizationColumns + ` FROM organizations ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()

	var orgs []*models.Organization
	for rows.Next() {
		org := &models.Organization{}
		var created, updated string
		if err := rows.Scan(&org.ID, &org.Name, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		org.CreatedAt = models.ParseSQLiteTime(created)
		org.UpdatedAt = models.ParseSQLiteTime(updated)
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

func (r *OrganizationRepo) Update(org *models.Organization) error {
	updated, now := models.NowUTC()
	query := `UPDATE organizations SET name = ?, updated_at = ? WHERE id = ?`
	if _, err := r.db.Exec(query, org.Name, updated, org.ID); err != nil {
		return fmt.Errorf("update organization: %w", err)
	}
	org.UpdatedAt = now
	return nil
}

// Delete drops the organization and detaches its members. The foreign key
// already handles the detachment; the explicit UPDATE is a belt-and-braces
// guard for connections opened without foreign_keys=ON.
func (r *OrganizationRepo) Delete(id string) error {
	if _, err := r.db.Exec(`UPDATE persons SET org_id = NULL WHERE org_id = ?`, id); err != nil {
		return fmt.Errorf("detach organization members: %w", err)
	}
	if _, err := r.db.Exec(`DELETE FROM organizations WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete organization: %w", err)
	}
	return nil
}
