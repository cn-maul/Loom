package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"relationship/internal/models"
)

type EventRepo struct {
	db *sql.DB
}

func NewEventRepo(db *sql.DB) *EventRepo {
	return &EventRepo{db: db}
}

const eventColumns = `id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises, record_type, channel, created_at, extraction_status, extraction_error, extracted_at, manually_edited, edited_at`

// Create stores the record and its attendance list in one transaction: a record
// saved with participants but no attendance rows would be invisible from everyone
// but its primary person.
func (r *EventRepo) Create(event *models.Event) error {
	created, now := models.NowUTC()
	if event.ExtractionStatus == "" {
		event.ExtractionStatus = models.ExtractionPending
	}
	if event.ExtractionStatus != models.ExtractionPending && event.ExtractedAt == nil {
		event.ExtractedAt = &now
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin event insert: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO events (id, person_id, raw_text, event_date, summary, my_feeling, their_reaction, promises,
			record_type, channel, created_at, extraction_status, extraction_error, extracted_at, manually_edited, edited_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	if _, err := tx.Exec(query, event.ID, nullIfEmpty(event.PersonID), event.RawText, event.EventDate,
		event.Summary, event.MyFeeling, event.TheirReaction, event.Promises,
		nullIfEmpty(event.RecordType), nullIfEmpty(event.Channel), created,
		event.ExtractionStatus, nullIfEmpty(event.ExtractionError), timeArg(event.ExtractedAt),
		event.ManuallyEdited, timeArg(event.EditedAt)); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	if err := replaceParticipants(tx, event.ID, event.Participants); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit event insert: %w", err)
	}
	event.CreatedAt = now
	return nil
}

// scanEvent reads one row. The extracted columns are nullable in the schema: a
// record is stored before the AI pipeline fills them in, and a manual insert may
// leave them unset entirely. person_id is nullable too, so a record survives the
// deletion of its primary person as long as other participants remain.
func scanEvent(row rowScanner) (*models.Event, error) {
	event := &models.Event{}
	var personID, summary, feeling, reaction, promises, recordType, channel, created sql.NullString
	var extractionStatus, extractionError, extractedAt, editedAt sql.NullString
	if err := row.Scan(
		&event.ID, &personID, &event.RawText, &event.EventDate,
		&summary, &feeling, &reaction, &promises, &recordType, &channel, &created,
		&extractionStatus, &extractionError, &extractedAt, &event.ManuallyEdited, &editedAt,
	); err != nil {
		return nil, err
	}
	event.PersonID = scanNullString(&personID)
	event.Summary = scanNullString(&summary)
	event.MyFeeling = scanNullString(&feeling)
	event.TheirReaction = scanNullString(&reaction)
	event.Promises = scanNullString(&promises)
	event.RecordType = scanNullString(&recordType)
	event.Channel = scanNullString(&channel)
	event.CreatedAt = models.ParseSQLiteTime(scanNullString(&created))
	event.ExtractionStatus = scanNullString(&extractionStatus)
	if event.ExtractionStatus == "" {
		// A row written before the column existed is at worst unprocessed, which
		// is what pending means.
		event.ExtractionStatus = models.ExtractionPending
	}
	event.ExtractionError = scanNullString(&extractionError)
	event.ExtractedAt = timePointer(scanNullString(&extractedAt))
	event.EditedAt = timePointer(scanNullString(&editedAt))
	return event, nil
}

func (r *EventRepo) GetByID(id string) (*models.Event, error) {
	event, err := scanEvent(r.db.QueryRow(`SELECT `+eventColumns+` FROM events WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.NewError(models.ErrNotFound, "event %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	if err := r.attachParticipants([]*models.Event{event}); err != nil {
		return nil, err
	}
	return event, nil
}

// ListByPerson returns every record the person attended, whether they are the
// primary person or a participant. The record is stored once and read from every
// attendee's timeline.
func (r *EventRepo) ListByPerson(personID string, limit int) ([]*models.Event, error) {
	if limit <= 0 {
		limit = 50
	}
	events, _, err := r.ListFiltered(models.EventFilter{PersonID: personID, Limit: limit})
	return events, err
}

// ListFiltered answers the record list query with the filters and page window the
// API accepts. It also reports the total number of matches, so a paginated caller
// knows how much is left without a second round trip. A limit of zero or less
// means no page window: everything matching comes back.
func (r *EventRepo) ListFiltered(filter models.EventFilter) ([]*models.Event, int, error) {
	where, args := eventFilterClause(filter)

	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM events e`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	queryArgs := args
	query := `SELECT ` + eventColumnsFor("e") + ` FROM events e` + where +
		` ORDER BY e.event_date DESC, e.created_at DESC`
	if filter.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		queryArgs = append(append([]any{}, args...), filter.Limit, filter.Offset)
	}

	rows, err := r.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, 0, err
	}
	if err := r.attachParticipants(events); err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// eventFilterClause builds the WHERE shared by the page query and its count, so
// the two can never drift apart and report a total that does not match the rows.
func eventFilterClause(filter models.EventFilter) (string, []any) {
	where := ` WHERE 1 = 1`
	args := []any{}
	if filter.PersonID != "" {
		where += ` AND (e.person_id = ? OR EXISTS (
			SELECT 1 FROM event_participants ep WHERE ep.event_id = e.id AND ep.person_id = ?))`
		args = append(args, filter.PersonID, filter.PersonID)
	}
	if filter.From != "" {
		where += ` AND e.event_date >= ?`
		args = append(args, filter.From)
	}
	if filter.To != "" {
		where += ` AND e.event_date <= ?`
		args = append(args, filter.To)
	}
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		where += ` AND (e.raw_text LIKE ? OR COALESCE(e.summary, '') LIKE ?)`
		args = append(args, like, like)
	}
	if filter.Status != "" {
		where += ` AND COALESCE(e.extraction_status, 'pending') = ?`
		args = append(args, filter.Status)
	}
	return where, args
}

func (r *EventRepo) ListAll() ([]*models.Event, error) {
	rows, err := r.db.Query(`SELECT ` + eventColumns + ` FROM events ORDER BY event_date ASC`)
	if err != nil {
		return nil, fmt.Errorf("list all events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (r *EventRepo) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM events WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return requireRow(res, "event", id)
}

// ContentRevs fingerprints the given records as they are stored right now. It is
// the comparison half of advice staleness: a stored answer keeps the fingerprint
// it was drawn from, and this reports what the records look like today. An id
// that no longer exists simply does not appear, which is how a deleted record is
// detected without a second query.
func (r *EventRepo) ContentRevs(ids []string) (map[string]string, error) {
	revs := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return revs, nil
	}
	placeholders, args := inClause(ids)
	rows, err := r.db.Query(`SELECT `+eventColumns+` FROM events WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("read event revisions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event revision: %w", err)
		}
		revs[event.ID] = models.EventContentRev(event)
	}
	return revs, rows.Err()
}

// Update writes the editable fields a hand edit may change, plus
// record_type/channel, and records that a human has now curated this record: an
// automated retry must not overwrite that without an explicit override.
// Attendance is managed separately so an edit cannot silently drop participants.
func (r *EventRepo) Update(event *models.Event) error {
	now, t := models.NowUTC()
	query := `
		UPDATE events SET raw_text = ?, event_date = ?, summary = ?, my_feeling = ?, their_reaction = ?,
			promises = ?, record_type = ?, channel = ?, manually_edited = 1, edited_at = ?
		WHERE id = ?
	`
	res, err := r.db.Exec(query, event.RawText, event.EventDate, event.Summary, event.MyFeeling,
		event.TheirReaction, event.Promises, nullIfEmpty(event.RecordType), nullIfEmpty(event.Channel),
		now, event.ID)
	if err != nil {
		return fmt.Errorf("update event: %w", err)
	}
	if err := requireRow(res, "event", event.ID); err != nil {
		return err
	}
	event.ManuallyEdited = 1
	event.EditedAt = &t
	return nil
}

// UpdateExtraction persists a successful extraction. It deliberately does not
// touch the raw text, and it clears the manual flag because the stored result is
// machine output again — which is exactly what an explicit force-retry asks for.
func (r *EventRepo) UpdateExtraction(event *models.Event) error {
	now, t := models.NowUTC()
	res, err := r.db.Exec(`
		UPDATE events SET summary = ?, my_feeling = ?, their_reaction = ?, promises = ?,
			extraction_status = ?, extraction_error = NULL, extracted_at = ?, manually_edited = 0
		WHERE id = ?`,
		event.Summary, event.MyFeeling, event.TheirReaction, event.Promises,
		models.ExtractionSucceeded, now, event.ID)
	if err != nil {
		return fmt.Errorf("update extraction: %w", err)
	}
	if err := requireRow(res, "event", event.ID); err != nil {
		return err
	}
	event.ExtractionStatus = models.ExtractionSucceeded
	event.ExtractionError = ""
	event.ExtractedAt = &t
	event.ManuallyEdited = 0
	return nil
}

// RecordExtractionFailure keeps whatever content the record already had and only
// reports the attempt: a failed retry must not blank out an earlier success or a
// hand-written summary.
func (r *EventRepo) RecordExtractionFailure(id, message string) error {
	now, _ := models.NowUTC()
	res, err := r.db.Exec(`
		UPDATE events SET extraction_status = ?, extraction_error = ?, extracted_at = ?
		WHERE id = ?`, models.ExtractionFailed, message, now, id)
	if err != nil {
		return fmt.Errorf("record extraction failure: %w", err)
	}
	return requireRow(res, "event", id)
}

// ListParticipants returns one record's attendance list with display names.
func (r *EventRepo) ListParticipants(eventID string) ([]models.EventParticipant, error) {
	rows, err := r.db.Query(`
		SELECT ep.event_id, ep.person_id, COALESCE(p.name, ''), COALESCE(ep.role, '')
		FROM event_participants ep
		LEFT JOIN persons p ON p.id = ep.person_id
		WHERE ep.event_id = ?
		ORDER BY ep.rowid`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list event participants: %w", err)
	}
	defer rows.Close()

	participants := []models.EventParticipant{}
	for rows.Next() {
		var p models.EventParticipant
		if err := rows.Scan(&p.EventID, &p.PersonID, &p.PersonName, &p.Role); err != nil {
			return nil, fmt.Errorf("scan event participant: %w", err)
		}
		participants = append(participants, p)
	}
	return participants, rows.Err()
}

// SetParticipants replaces the attendance list. A record must keep at least one
// person, and the primary person follows the list when they are removed from it.
func (r *EventRepo) SetParticipants(eventID string, participants []models.EventParticipant) error {
	clean, err := normalizeParticipantList(participants)
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin participant update: %w", err)
	}
	defer tx.Rollback()

	var primary sql.NullString
	err = tx.QueryRow(`SELECT person_id FROM events WHERE id = ?`, eventID).Scan(&primary)
	if errors.Is(err, sql.ErrNoRows) {
		return models.NewError(models.ErrNotFound, "event %s not found", eventID)
	}
	if err != nil {
		return fmt.Errorf("read event %s: %w", eventID, err)
	}

	if err := replaceParticipants(tx, eventID, clean); err != nil {
		return err
	}
	if current := scanNullString(&primary); !containsPerson(clean, current) {
		if _, err := tx.Exec(`UPDATE events SET person_id = ? WHERE id = ?`, clean[0].PersonID, eventID); err != nil {
			return fmt.Errorf("repoint primary person: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit participant update: %w", err)
	}
	return nil
}

// attachParticipants fills Participants for a batch of records with a single
// query, so a list of N records does not cost N lookups.
func (r *EventRepo) attachParticipants(events []*models.Event) error {
	if len(events) == 0 {
		return nil
	}
	ids := make([]string, 0, len(events))
	byID := make(map[string]*models.Event, len(events))
	for _, e := range events {
		e.Participants = []models.EventParticipant{}
		ids = append(ids, e.ID)
		byID[e.ID] = e
	}
	placeholders, args := inClause(ids)
	rows, err := r.db.Query(`
		SELECT ep.event_id, ep.person_id, COALESCE(p.name, ''), COALESCE(ep.role, '')
		FROM event_participants ep
		LEFT JOIN persons p ON p.id = ep.person_id
		LEFT JOIN events e ON e.id = ep.event_id
		WHERE ep.event_id IN (`+placeholders+`)
		ORDER BY ep.event_id, CASE WHEN ep.person_id = e.person_id THEN 0 ELSE 1 END, ep.rowid`, args...)
	if err != nil {
		return fmt.Errorf("list event participants: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p models.EventParticipant
		if err := rows.Scan(&p.EventID, &p.PersonID, &p.PersonName, &p.Role); err != nil {
			return fmt.Errorf("scan event participant: %w", err)
		}
		if event := byID[p.EventID]; event != nil {
			event.Participants = append(event.Participants, p)
		}
	}
	return rows.Err()
}

// replaceParticipants clears and rewrites one record's attendance inside an
// existing transaction.
func replaceParticipants(x execer, eventID string, participants []models.EventParticipant) error {
	if _, err := x.Exec(`DELETE FROM event_participants WHERE event_id = ?`, eventID); err != nil {
		return fmt.Errorf("clear event participants: %w", err)
	}
	for _, p := range participants {
		if _, err := x.Exec(`
			INSERT INTO event_participants (event_id, person_id, role) VALUES (?, ?, ?)
			ON CONFLICT(event_id, person_id) DO UPDATE SET role = excluded.role`,
			eventID, p.PersonID, nullIfEmpty(strings.TrimSpace(p.Role))); err != nil {
			return fmt.Errorf("insert event participant %s: %w", p.PersonID, err)
		}
	}
	return nil
}

// normalizeParticipantList dedupes and rejects an empty attendance list, which
// would leave a record nobody can open.
func normalizeParticipantList(participants []models.EventParticipant) ([]models.EventParticipant, error) {
	seen := map[string]bool{}
	clean := []models.EventParticipant{}
	for _, p := range participants {
		id := strings.TrimSpace(p.PersonID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		p.PersonID = id
		p.Role = strings.TrimSpace(p.Role)
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return nil, models.NewError(models.ErrInvalidInput, "at least one participant is required")
	}
	return clean, nil
}

func containsPerson(participants []models.EventParticipant, personID string) bool {
	if personID == "" {
		return false
	}
	for _, p := range participants {
		if p.PersonID == personID {
			return true
		}
	}
	return false
}

func scanEvents(rows *sql.Rows) ([]*models.Event, error) {
	var events []*models.Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
