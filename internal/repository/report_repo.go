package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"relationship/internal/models"
)

type ReportRepo struct{ db *sql.DB }

func NewReportRepo(db *sql.DB) *ReportRepo { return &ReportRepo{db: db} }

// reportSnapshotColumns is what the history list needs. payload is deliberately
// absent: a year of snapshots should not be one query that ships a year of
// sections to render a list of dates.
const reportSnapshotColumns = `id, person_id, person_name, start_date, end_date, status,
	generated_by, failure_reason, summary, event_count, generated_at`

// Create stores one report snapshot. A snapshot is written once and never
// updated: it records what the report said, which is the only way an old report
// can stay read later instead of being silently recomputed against new data.
func (r *ReportRepo) Create(report *models.Report) error {
	// Stamp before encoding: the payload is what Get returns, so the column and
	// the payload would otherwise disagree about when this report was made.
	now, t := models.NowUTC()
	if report.GeneratedAt.IsZero() {
		report.GeneratedAt = t
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode report payload: %w", err)
	}

	_, err = r.db.Exec(`
		INSERT INTO report_snapshots (id, person_id, person_name, start_date, end_date, status,
			generated_by, failure_reason, summary, event_count, payload, generated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.ID, nullIfEmpty(report.PersonID), nullIfEmpty(report.PersonName),
		report.Start, report.End, report.Status,
		nullIfEmpty(report.GeneratedBy), nullIfEmpty(report.FailureReason),
		report.Summary, report.EventCount, string(payload), now)
	if err != nil {
		return fmt.Errorf("create report snapshot: %w", err)
	}
	return nil
}

// Get returns one stored report with its sections, newest state intact.
func (r *ReportRepo) Get(id string) (*models.Report, error) {
	var payload sql.NullString
	err := r.db.QueryRow(`SELECT payload FROM report_snapshots WHERE id = ?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.NewError(models.ErrNotFound, "report %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get report: %w", err)
	}

	report := &models.Report{}
	if err := json.Unmarshal([]byte(scanNullString(&payload)), report); err != nil {
		return nil, fmt.Errorf("decode report %s: %w", id, err)
	}
	report.ID = id
	return report, nil
}

// List returns the report history newest first. An empty personID is the
// all-people view.
func (r *ReportRepo) List(personID string, limit, offset int) ([]*models.ReportSnapshot, error) {
	query := `SELECT ` + reportSnapshotColumns + ` FROM report_snapshots WHERE 1 = 1`
	args := []any{}
	if personID != "" {
		query += ` AND person_id = ?`
		args = append(args, personID)
	}
	query += ` ORDER BY generated_at DESC, rowid DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()

	result := []*models.ReportSnapshot{}
	for rows.Next() {
		snapshot, err := scanReportSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("scan report snapshot: %w", err)
		}
		result = append(result, snapshot)
	}
	return result, rows.Err()
}

func (r *ReportRepo) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM report_snapshots WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete report: %w", err)
	}
	return requireRow(res, "report", id)
}

func scanReportSnapshot(row rowScanner) (*models.ReportSnapshot, error) {
	s := &models.ReportSnapshot{}
	var personID, personName, generatedBy, failureReason, summary, generatedAt sql.NullString
	if err := row.Scan(
		&s.ID, &personID, &personName, &s.Start, &s.End, &s.Status,
		&generatedBy, &failureReason, &summary, &s.EventCount, &generatedAt,
	); err != nil {
		return nil, err
	}
	s.PersonID = scanNullString(&personID)
	s.PersonName = scanNullString(&personName)
	s.GeneratedBy = scanNullString(&generatedBy)
	s.FailureReason = scanNullString(&failureReason)
	s.Summary = scanNullString(&summary)
	s.GeneratedAt = models.ParseSQLiteTime(scanNullString(&generatedAt))
	return s, nil
}
