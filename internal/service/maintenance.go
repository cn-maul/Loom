package service

import (
	"database/sql"
	"fmt"
	"time"

	"relationship/internal/repository"
)

// CleanupReport is the result of one maintenance run. Every counter is
// reported, including zeros: "nothing was cleaned" must be distinguishable
// from "the check never ran".
type CleanupReport struct {
	VectorsChecked     int `json:"vectors_checked"`
	OrphanVectors      int `json:"orphan_vectors_removed"`
	AuditRowsRemoved   int `json:"audit_rows_removed"`
	AuditRetentionDays int `json:"audit_retention_days"`
}

// MaintenanceService owns the periodic data-hygiene pass: it removes vector
// rows whose facts no longer exist (orphaned by a crash between a delete and
// its index cleanup) and trims the audit log to its retention window. Failed
// extraction tasks need no cleaner — the queue history self-prunes at 500
// entries, and a failed record keeps its raw text, which is a fact, not waste.
type MaintenanceService struct {
	db                 *sql.DB
	vec                *repository.VecRepo
	auditRetentionDays int
}

// NewMaintenanceService wires the cleaner. auditRetentionDays of 0 (or less)
// disables audit trimming — keeping security evidence forever is the safe
// default; shortening it is a deliberate config choice.
func NewMaintenanceService(db *sql.DB, vec *repository.VecRepo, auditRetentionDays int) *MaintenanceService {
	return &MaintenanceService{db: db, vec: vec, auditRetentionDays: auditRetentionDays}
}

// Cleanup runs one full pass and reports what it did. Vector hygiene is
// skipped (not failed) when the index is unavailable: the report shows
// VectorsChecked=0, which is the honest answer.
func (s *MaintenanceService) Cleanup() (*CleanupReport, error) {
	report := &CleanupReport{AuditRetentionDays: s.auditRetentionDays}

	if s.vec.Available() {
		removed, checked, err := s.cleanOrphanVectors()
		if err != nil {
			return report, fmt.Errorf("clean orphan vectors: %w", err)
		}
		report.OrphanVectors = removed
		report.VectorsChecked = checked
	}

	removed, err := s.cleanAuditLog()
	if err != nil {
		return report, fmt.Errorf("trim audit log: %w", err)
	}
	report.AuditRowsRemoved = removed
	return report, nil
}

// cleanOrphanVectors drops vec_memory rows that no live fact can reach:
// a person_id pointing at a deleted person, or a source_id pointing at a
// deleted event/trait. Deletes go one by one — personal scale makes batch
// cleverness worthless, and per-row errors stay attributable.
func (s *MaintenanceService) cleanOrphanVectors() (removed, checked int, err error) {
	persons, err := s.idSet(`SELECT id FROM persons`)
	if err != nil {
		return 0, 0, err
	}
	events, err := s.idSet(`SELECT id FROM events`)
	if err != nil {
		return 0, 0, err
	}
	traits, err := s.idSet(`SELECT id FROM traits`)
	if err != nil {
		return 0, 0, err
	}

	rows, err := s.db.Query(`SELECT id, person_id, source_id, chunk_type FROM vec_memory`)
	if err != nil {
		return 0, 0, fmt.Errorf("scan vec_memory: %w", err)
	}
	defer rows.Close()

	var orphanIDs []string
	for rows.Next() {
		var id, personID, sourceID, chunkType string
		if err := rows.Scan(&id, &personID, &sourceID, &chunkType); err != nil {
			return 0, 0, fmt.Errorf("scan vec row: %w", err)
		}
		checked++
		if s.isOrphan(personID, sourceID, chunkType, persons, events, traits) {
			orphanIDs = append(orphanIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, checked, fmt.Errorf("iterate vec_memory: %w", err)
	}

	for _, id := range orphanIDs {
		if _, err := s.db.Exec(`DELETE FROM vec_memory WHERE id = ?`, id); err != nil {
			return removed, checked, fmt.Errorf("delete orphan vector %s: %w", id, err)
		}
		removed++
	}
	return removed, checked, nil
}

// isOrphan decides for one vector row. Unknown chunk types are left alone:
// a future chunk type with a different source table must not be deleted by
// a cleaner that does not understand it.
func (s *MaintenanceService) isOrphan(personID, sourceID, chunkType string, persons, events, traits map[string]bool) bool {
	if personID != "" && !persons[personID] {
		return true
	}
	switch chunkType {
	case "event_summary":
		return sourceID != "" && !events[sourceID]
	case "trait":
		return sourceID != "" && !traits[sourceID]
	}
	return false
}

// cleanAuditLog removes audit rows older than the retention window. The
// created_at column is SQLite datetime('now') format, so the cutoff uses the
// same shape — an RFC3339 string would never match anything.
func (s *MaintenanceService) cleanAuditLog() (int, error) {
	if s.auditRetentionDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -s.auditRetentionDays).Format("2006-01-02 15:04:05")
	res, err := s.db.Exec(`DELETE FROM audit_log WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete expired audit rows: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (s *MaintenanceService) idSet(query string) (map[string]bool, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", query, err)
	}
	defer rows.Close()
	set := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		set[id] = true
	}
	return set, rows.Err()
}
