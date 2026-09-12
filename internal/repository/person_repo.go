package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"relationship/internal/models"
)

type PersonRepo struct {
	db *sql.DB
}

func NewPersonRepo(db *sql.DB) *PersonRepo {
	return &PersonRepo{db: db}
}

const personColumns = `id, name, relation, importance, notes, org_id, position, gender, is_self, created_at, updated_at`

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
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin person insert: %w", err)
	}
	defer tx.Rollback()

	// Clear the previous holder of the self flag first: the partial unique index
	// on persons(is_self) would otherwise reject the insert.
	if person.IsSelf == 1 {
		if err := clearSelf(tx, person.ID); err != nil {
			return err
		}
	}
	query := `
		INSERT INTO persons (id, name, relation, importance, notes, org_id, position, gender, is_self, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	if _, err := tx.Exec(query, person.ID, person.Name, person.Relation, person.Importance, person.Notes,
		nullIfEmpty(person.OrgID), nullIfEmpty(person.Position), nullIfEmpty(person.Gender), person.IsSelf, created, created); err != nil {
		return fmt.Errorf("insert person: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit person insert: %w", err)
	}
	person.CreatedAt, person.UpdatedAt = now, now
	return nil
}

func (r *PersonRepo) GetByID(id string) (*models.Person, error) {
	person := &models.Person{}
	var created, updated string
	// relation and notes are nullable without a default, so they have to be
	// scanned through sql.NullString or a row stored with NULL fails to read.
	var relation, notes, orgID, position, gender sql.NullString
	err := r.db.QueryRow(`SELECT `+personColumns+` FROM persons WHERE id = ?`, id).Scan(
		&person.ID, &person.Name, &relation, &person.Importance,
		&notes, &orgID, &position, &gender, &person.IsSelf, &created, &updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.NewError(models.ErrNotFound, "person %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get person: %w", err)
	}
	person.Relation = scanNullString(&relation)
	person.Notes = scanNullString(&notes)
	person.OrgID = scanNullString(&orgID)
	person.Position = scanNullString(&position)
	person.Gender = scanNullString(&gender)
	person.CreatedAt = models.ParseSQLiteTime(created)
	person.UpdatedAt = models.ParseSQLiteTime(updated)
	return person, nil
}

// ListWithActivity returns every person with the timeline metadata the home page
// needs, ordered by most recent interaction rather than by record edits. A record
// counts for someone who attended it as a participant, not only as the primary
// person, which is what makes a shared record show up on every attendee's card.
func (r *PersonRepo) ListWithActivity() ([]*models.PersonWithActivity, error) {
	persons, _, err := r.ListFiltered(models.PersonFilter{})
	return persons, err
}

// ListFiltered answers the person list query with the keyword, affiliation,
// relation, ordering and page window the API accepts. It also reports how many
// persons match the filters in total, so a paginated caller knows how much is
// left without a second round trip. A limit of zero or less means no page
// window: everything matching comes back.
func (r *PersonRepo) ListFiltered(filter models.PersonFilter) ([]*models.PersonWithActivity, int, error) {
	// Every filter references persons columns only, so the same WHERE counts
	// the matches without dragging the activity join along.
	where := " WHERE 1 = 1"
	args := []any{}
	if filter.Q != "" {
		like := "%" + filter.Q + "%"
		where += ` AND (p.name LIKE ? OR COALESCE(p.relation, '') LIKE ? OR COALESCE(p.position, '') LIKE ? OR COALESCE(p.notes, '') LIKE ?)`
		args = append(args, like, like, like, like)
	}
	if filter.OrgID == "none" {
		where += " AND p.org_id IS NULL"
	} else if filter.OrgID != "" {
		where += " AND p.org_id = ?"
		args = append(args, filter.OrgID)
	}
	if filter.Relation != "" {
		where += " AND COALESCE(p.relation, '') = ?"
		args = append(args, filter.Relation)
	}

	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM persons p`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count persons: %w", err)
	}

	query := `
		SELECT p.id, p.name, p.relation, p.importance, p.notes, p.org_id, p.position, p.gender, p.is_self, p.created_at, p.updated_at,
		       o.name AS org_name,
		       MAX(e.event_date) AS last_event_date, COUNT(DISTINCT e.id) AS event_count
		FROM persons p
		LEFT JOIN organizations o ON o.id = p.org_id
		LEFT JOIN events e ON e.person_id = p.id
		    OR e.id IN (SELECT ep.event_id FROM event_participants ep WHERE ep.person_id = p.id)
	` + where + ` GROUP BY p.id` + personOrderBy(filter.Sort)
	queryArgs := args
	if filter.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		queryArgs = append(append([]any{}, args...), filter.Limit, filter.Offset)
	}

	rows, err := r.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list persons: %w", err)
	}
	defer rows.Close()

	var persons []*models.PersonWithActivity
	for rows.Next() {
		person := &models.PersonWithActivity{}
		var created, updated string
		var relation, notes, orgID, position, gender, lastEvent, orgName sql.NullString
		if err := rows.Scan(
			&person.ID, &person.Name, &relation, &person.Importance,
			&notes, &orgID, &position, &gender, &person.IsSelf, &created, &updated,
			&orgName, &lastEvent, &person.EventCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan person: %w", err)
		}
		person.Relation = scanNullString(&relation)
		person.Notes = scanNullString(&notes)
		person.OrgID = scanNullString(&orgID)
		person.Position = scanNullString(&position)
		person.Gender = scanNullString(&gender)
		person.CreatedAt = models.ParseSQLiteTime(created)
		person.UpdatedAt = models.ParseSQLiteTime(updated)
		person.LastEventDate = scanNullString(&lastEvent)
		person.OrgName = scanNullString(&orgName)
		persons = append(persons, person)
	}
	return persons, total, rows.Err()
}

// personOrderBy maps the closed ordering vocabulary to SQL. The default keeps
// the historical behaviour: most recent interaction first, creation date as a
// fallback for persons without records.
func personOrderBy(sort string) string {
	switch sort {
	case models.PersonSortName:
		return ` ORDER BY p.name COLLATE NOCASE ASC`
	case models.PersonSortImportance:
		return ` ORDER BY p.importance DESC, COALESCE(MAX(e.event_date), p.created_at) DESC`
	case models.PersonSortCreated:
		return ` ORDER BY p.created_at DESC`
	default:
		return ` ORDER BY COALESCE(MAX(e.event_date), p.created_at) DESC`
	}
}

func (r *PersonRepo) Update(person *models.Person) error {
	updated, now := models.NowUTC()
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin person update: %w", err)
	}
	defer tx.Rollback()

	if person.IsSelf == 1 {
		if err := clearSelf(tx, person.ID); err != nil {
			return err
		}
	}
	query := `
		UPDATE persons SET name = ?, relation = ?, importance = ?, notes = ?, org_id = ?, position = ?, gender = ?, is_self = ?, updated_at = ?
		WHERE id = ?
	`
	res, err := tx.Exec(query, person.Name, person.Relation, person.Importance, person.Notes,
		nullIfEmpty(person.OrgID), nullIfEmpty(person.Position), nullIfEmpty(person.Gender), person.IsSelf, updated, person.ID)
	if err != nil {
		return fmt.Errorf("update person: %w", err)
	}
	if err := requireRow(res, "person", person.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit person update: %w", err)
	}
	person.UpdatedAt = now
	return nil
}

func (r *PersonRepo) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM persons WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete person: %w", err)
	}
	return requireRow(res, "person", id)
}

// clearSelf demotes every other person, so at most one row carries is_self.
func clearSelf(x execer, exceptID string) error {
	if _, err := x.Exec(`UPDATE persons SET is_self = 0 WHERE is_self = 1 AND id <> ?`, exceptID); err != nil {
		return fmt.Errorf("clear previous self person: %w", err)
	}
	return nil
}
