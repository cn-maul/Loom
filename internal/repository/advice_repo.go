package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"relationship/internal/models"
)

type AdviceRepo struct{ db *sql.DB }

func NewAdviceRepo(db *sql.DB) *AdviceRepo { return &AdviceRepo{db: db} }

const adviceColumns = `id, person_id, question, goal, answer, used_event_ids, used_trait_ids,
	evidence_version, retrieval_status, model, adopted_strategy_index, adopted_strategy_name,
	created_at, updated_at`

// Create stores one answered advice session. The generated answer is kept as a
// single JSON document because it is always read as a whole; the fields that are
// actually queried — person, time, retrieval outcome — are real columns.
func (r *AdviceRepo) Create(s *models.AdviceSession) error {
	answer, err := json.Marshal(s.AdviceResponse)
	if err != nil {
		return fmt.Errorf("encode advice answer: %w", err)
	}
	usedEvents := models.EncodeIDList(s.UsedEventIDs)
	usedTraits := models.EncodeIDList(s.UsedTraitIDs)
	versions, err := json.Marshal(nonNilVersions(s.EvidenceVersion))
	if err != nil {
		return fmt.Errorf("encode evidence versions: %w", err)
	}

	now, t := models.NowUTC()
	_, err = r.db.Exec(`
		INSERT INTO advice_sessions (id, person_id, question, goal, answer, used_event_ids, used_trait_ids,
			evidence_version, retrieval_status, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.PersonID, s.Question, nullIfEmpty(s.Goal), string(answer), usedEvents, usedTraits,
		string(versions), nullIfEmpty(s.RetrievalStatus), nullIfEmpty(s.Model), now, now)
	if err != nil {
		return fmt.Errorf("create advice session: %w", err)
	}
	s.CreatedAt, s.UpdatedAt = t, t
	return nil
}

func (r *AdviceRepo) Get(id string) (*models.AdviceSession, error) {
	s, err := scanAdviceSession(r.db.QueryRow(`SELECT `+adviceColumns+` FROM advice_sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.NewError(models.ErrNotFound, "advice session %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get advice session: %w", err)
	}
	return s, nil
}

// List returns a person's advice history newest first. An empty personID is the
// all-people view, which is what the advice page falls back to.
func (r *AdviceRepo) List(personID string, limit, offset int) ([]*models.AdviceSession, error) {
	query := `SELECT ` + adviceColumns + ` FROM advice_sessions WHERE 1 = 1`
	args := []any{}
	if personID != "" {
		query += ` AND person_id = ?`
		args = append(args, personID)
	}
	query += ` ORDER BY created_at DESC, rowid DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list advice sessions: %w", err)
	}
	defer rows.Close()

	result := []*models.AdviceSession{}
	for rows.Next() {
		s, err := scanAdviceSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan advice session: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// Delete removes a session. Referencing follow-ups are invalidated by the
// service before this runs, because the foreign key nulls the link and would
// otherwise erase the only trace of where the item came from.
func (r *AdviceRepo) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM advice_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete advice session: %w", err)
	}
	return requireRow(res, "advice session", id)
}

// SetAdopted records which strategy the user decided to act on, so the history
// shows a decision rather than only a suggestion.
func (r *AdviceRepo) SetAdopted(id string, index int, name string) error {
	now, _ := models.NowUTC()
	res, err := r.db.Exec(`
		UPDATE advice_sessions SET adopted_strategy_index = ?, adopted_strategy_name = ?, updated_at = ?
		WHERE id = ?`, index, name, now, id)
	if err != nil {
		return fmt.Errorf("record adopted strategy: %w", err)
	}
	return requireRow(res, "advice session", id)
}

func scanAdviceSession(row rowScanner) (*models.AdviceSession, error) {
	s := &models.AdviceSession{}
	var goal, answer, usedEvents, usedTraits, versions, retrievalStatus, model, adoptedName sql.NullString
	var adoptedIndex sql.NullInt64
	var created, updated sql.NullString
	if err := row.Scan(
		&s.ID, &s.PersonID, &s.Question, &goal, &answer, &usedEvents, &usedTraits,
		&versions, &retrievalStatus, &model, &adoptedIndex, &adoptedName, &created, &updated,
	); err != nil {
		return nil, err
	}
	s.Goal = scanNullString(&goal)
	if raw := scanNullString(&answer); raw != "" {
		if err := json.Unmarshal([]byte(raw), &s.AdviceResponse); err != nil {
			return nil, fmt.Errorf("decode advice answer for %s: %w", s.ID, err)
		}
	}
	s.UsedEventIDs = decodeIDList(scanNullString(&usedEvents))
	s.UsedTraitIDs = decodeIDList(scanNullString(&usedTraits))
	s.EvidenceVersion = decodeEvidenceVersions(scanNullString(&versions))
	s.RetrievalStatus = scanNullString(&retrievalStatus)
	s.VectorUsed = s.RetrievalStatus == models.RetrievalVectorUsed
	s.Model = scanNullString(&model)
	if adoptedIndex.Valid {
		index := int(adoptedIndex.Int64)
		s.AdoptedStrategyIndex = &index
	}
	s.AdoptedStrategyName = scanNullString(&adoptedName)
	s.CreatedAt = models.ParseSQLiteTime(scanNullString(&created))
	s.UpdatedAt = models.ParseSQLiteTime(scanNullString(&updated))
	// Derived on read, so it starts as an empty list rather than nil: a JSON
	// null here would make every consumer guard against it.
	s.FollowUpIDs = []string{}
	return s, nil
}

// decodeIDList tolerates both the JSON array form and the legacy comma form,
// mirroring the trait columns.
func decodeIDList(raw string) []string {
	out := []string{}
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func decodeEvidenceVersions(raw string) []models.EvidenceVersion {
	out := []models.EvidenceVersion{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func nonNilVersions(versions []models.EvidenceVersion) []models.EvidenceVersion {
	if versions == nil {
		return []models.EvidenceVersion{}
	}
	return versions
}
