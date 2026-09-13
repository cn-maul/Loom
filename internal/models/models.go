package models

import (
	"encoding/json"
	"strings"
	"time"
)

// SQLiteTimeLayout is the TEXT format datetime('now') produces; the schema keeps
// timestamps as text in this layout.
const SQLiteTimeLayout = "2006-01-02 15:04:05"

// NowUTC returns the current instant as both the stored TEXT and the parsed value, so
// a create can write its own timestamps instead of reading the schema defaults back.
func NowUTC() (string, time.Time) {
	now := time.Now().UTC().Truncate(time.Second)
	return now.Format(SQLiteTimeLayout), now
}

// ParseSQLiteTime reads the TEXT timestamps this schema stores. datetime('now')
// yields a space-separated UTC value that is not RFC3339, so the driver hands it
// over as a plain string that database/sql will not convert on its own.
func ParseSQLiteTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Kind is the organisation type in the user's own words (公司 / 学校 / 社团…);
	// free text rather than a closed vocabulary, like person.relation.
	Kind        string `json:"kind"`
	Description string `json:"description"`
	// ArchivedAt is nil for an active organisation. Archived ones stay readable
	// — past postings still reference them — but stop appearing in pickers.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (o *Organization) Validate() error {
	if strings.TrimSpace(o.Name) == "" {
		return NewError(ErrInvalidInput, "name is required")
	}
	return nil
}

type Person struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Relation   string `json:"relation"`
	Importance int    `json:"importance"`
	Notes      string `json:"notes"`
	OrgID      string `json:"org_id"`
	Position   string `json:"position"`
	// Gender is an explicit choice the user records: male / female, empty when
	// not set. It is a fact about the person, not an inference.
	Gender string `json:"gender"`
	// IsSelf marks the person the user is themselves, so relationship edges can
	// be phrased from "me" without guessing which node is the subject.
	IsSelf    int       `json:"is_self"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PersonWithActivity adds the list columns the home page sorts and displays by.
type PersonWithActivity struct {
	Person
	LastEventDate string `json:"last_event_date"`
	EventCount    int    `json:"event_count"`
	// OrgName is denormalized from the joined organizations row so the list
	// can show the affiliation without a second lookup per person.
	OrgName string `json:"org_name"`
}

// PersonFilter narrows the person list: keyword search, affiliation, relation,
// an ordering choice and the page window. OrgID accepts the sentinel "none"
// for persons without an organization.
type PersonFilter struct {
	Q        string
	OrgID    string
	Relation string
	Sort     string
	Limit    int
	Offset   int
}

// Person list orderings. The set is closed: the service rejects anything else
// rather than silently falling back to one.
const (
	PersonSortRecent     = "recent"
	PersonSortName       = "name"
	PersonSortImportance = "importance"
	PersonSortCreated    = "created"
)

// Follow-up statuses. They are stored verbatim, so the set is closed and shared
// by the service, the repository and the API.
const (
	FollowUpPending   = "pending"
	FollowUpWaiting   = "waiting"
	FollowUpCompleted = "completed"
	FollowUpCancelled = "cancelled"
)

type FollowUp struct {
	ID          string `json:"id"`
	PersonID    string `json:"person_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Owner says whose move it is ("我" / "对方"). Free text, because a fixed
	// vocabulary would not fit every relationship.
	Owner string `json:"owner"`
	// DueText preserves the wording from the record ("下周三前") while DueDate is
	// the confirmed date that sorting and filtering actually use. Keeping both
	// means a vague promise is still storable and still searchable.
	DueText string `json:"due_text"`
	DueDate string `json:"due_date"`
	Status  string `json:"status"`
	// SourceEventID is the record the commitment came out of. Empty for an item
	// typed by hand; NULL in the schema, so deleting the record detaches the
	// link instead of destroying the follow-up.
	SourceEventID string `json:"source_event_id"`
	// CompletionNote and CompletedEventID describe the outcome, not just the
	// state: what actually happened, and the record written afterwards.
	CompletionNote   string `json:"completion_note"`
	CompletedEventID string `json:"completed_event_id"`
	// SourceAdviceID is the advice session this item came out of. Unlike
	// SourceEventID it keeps a marker when the source goes away: an adopted
	// strategy is a premise for the action, and "the reasoning behind this is
	// gone" is worth showing on the open item itself.
	SourceAdviceID          string    `json:"source_advice_id"`
	SourceAdviceStale       int       `json:"source_advice_stale"`
	SourceAdviceStaleReason string    `json:"source_advice_stale_reason"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	// CompletedAt is a pointer so encoding/json can actually omit it: the
	// omitempty tag is ignored for struct values, which made every open item
	// serialise as "0001-01-01T00:00:00Z".
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// Postponements is the deadline history, filled on a single read. List
	// responses leave it empty rather than paying one query per row.
	Postponements []Postponement `json:"postponements,omitempty"`
}

// Postponement is one recorded move of a deadline, kept as history: the item
// itself only stores the latest date, so without this a postponed deadline
// silently replaced the original one.
type Postponement struct {
	ID         string    `json:"id"`
	FollowUpID string    `json:"follow_up_id"`
	OldDueDate string    `json:"old_due_date"`
	NewDueDate string    `json:"new_due_date"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

func (f *FollowUp) Validate() error {
	if f.PersonID == "" || strings.TrimSpace(f.Title) == "" {
		return NewError(ErrInvalidInput, "person_id and title are required")
	}
	if f.DueDate != "" {
		if _, err := time.Parse("2006-01-02", f.DueDate); err != nil {
			return NewError(ErrInvalidInput, "due_date must be YYYY-MM-DD")
		}
	}
	if f.Status == "" {
		f.Status = FollowUpPending
	}
	for _, s := range []string{FollowUpPending, FollowUpWaiting, FollowUpCompleted, FollowUpCancelled} {
		if f.Status == s {
			return nil
		}
	}
	return NewError(ErrInvalidInput, "invalid status %q", f.Status)
}

// Extraction statuses for the AI pipeline behind a record. "pending" means the
// record exists but has never come back from the extractor, which is the honest
// state after a gateway failure — not a silent empty summary.
const (
	ExtractionPending   = "pending"
	ExtractionSucceeded = "succeeded"
	ExtractionFailed    = "failed"
)

type Event struct {
	ID        string `json:"id"`
	PersonID  string `json:"person_id"`
	RawText   string `json:"raw_text"`
	EventDate string `json:"event_date"`
	Summary   string `json:"summary"`
	// RecordType and Channel describe how the record happened (meeting, call,
	// message) and where (in person, WeChat), which the timeline groups by.
	RecordType    string    `json:"record_type"`
	Channel       string    `json:"channel"`
	MyFeeling     string    `json:"my_feeling"`
	TheirReaction string    `json:"their_reaction"`
	Promises      string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
	// ExtractionStatus/Error/At report what the pipeline managed to do, so the
	// UI can offer a retry instead of showing an empty summary as a result.
	ExtractionStatus string     `json:"extraction_status"`
	ExtractionError  string     `json:"extraction_error"`
	ExtractedAt      *time.Time `json:"extracted_at,omitempty"`
	// PipelineWarnings keeps the non-fatal pipeline complaints (skipped vector
	// indexing, failed profile refresh) from the last run, so "succeeded" can
	// be shown honestly as "succeeded with warnings" instead of hiding them.
	// Stored as a JSON array, exposed the same way promises are.
	PipelineWarnings string `json:"-"`
	// ManuallyEdited marks a record whose stored extraction was written by a
	// human. A retry must not overwrite that without an explicit force.
	ManuallyEdited int        `json:"manually_edited"`
	EditedAt       *time.Time `json:"edited_at,omitempty"`
	// Participants is the full attendance list; PersonID stays the primary
	// person for compatibility with records stored before multi-person support.
	Participants []EventParticipant `json:"participants"`
}

type EventPromise struct {
	Who      string `json:"who"`
	What     string `json:"what"`
	Deadline string `json:"deadline"`
}

// MarshalJSON exposes promises as a JSON array; the column keeps the raw text.
func (e Event) MarshalJSON() ([]byte, error) {
	var promises []EventPromise
	if e.Promises != "" {
		_ = json.Unmarshal([]byte(e.Promises), &promises)
	}
	if promises == nil {
		promises = []EventPromise{}
	}
	participants := e.Participants
	if participants == nil {
		participants = []EventParticipant{}
	}
	var warnings []string
	if e.PipelineWarnings != "" {
		_ = json.Unmarshal([]byte(e.PipelineWarnings), &warnings)
	}
	if warnings == nil {
		warnings = []string{}
	}
	return json.Marshal(struct {
		ID               string             `json:"id"`
		PersonID         string             `json:"person_id"`
		RawText          string             `json:"raw_text"`
		EventDate        string             `json:"event_date"`
		Summary          string             `json:"summary"`
		RecordType       string             `json:"record_type"`
		Channel          string             `json:"channel"`
		MyFeeling        string             `json:"my_feeling"`
		TheirReaction    string             `json:"their_reaction"`
		Promises         []EventPromise     `json:"promises"`
		CreatedAt        time.Time          `json:"created_at"`
		ExtractionStatus string             `json:"extraction_status"`
		ExtractionError  string             `json:"extraction_error"`
		ExtractedAt      *time.Time         `json:"extracted_at,omitempty"`
		ManuallyEdited   int                `json:"manually_edited"`
		EditedAt         *time.Time         `json:"edited_at,omitempty"`
		Participants     []EventParticipant `json:"participants"`
		PipelineWarnings []string           `json:"pipeline_warnings"`
	}{e.ID, e.PersonID, e.RawText, e.EventDate, e.Summary, e.RecordType, e.Channel,
		e.MyFeeling, e.TheirReaction, promises, e.CreatedAt,
		e.ExtractionStatus, e.ExtractionError, e.ExtractedAt, e.ManuallyEdited, e.EditedAt,
		participants, warnings})
}

// UnmarshalJSON accepts promises both as an array and as the string form.
func (e *Event) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID               string             `json:"id"`
		PersonID         string             `json:"person_id"`
		RawText          string             `json:"raw_text"`
		EventDate        string             `json:"event_date"`
		Summary          string             `json:"summary"`
		RecordType       string             `json:"record_type"`
		Channel          string             `json:"channel"`
		MyFeeling        string             `json:"my_feeling"`
		TheirReaction    string             `json:"their_reaction"`
		Promises         json.RawMessage    `json:"promises"`
		CreatedAt        time.Time          `json:"created_at"`
		ExtractionStatus string             `json:"extraction_status"`
		ExtractionError  string             `json:"extraction_error"`
		ExtractedAt      *time.Time         `json:"extracted_at"`
		ManuallyEdited   int                `json:"manually_edited"`
		EditedAt         *time.Time         `json:"edited_at"`
		Participants     []EventParticipant `json:"participants"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.ID = raw.ID
	e.PersonID = raw.PersonID
	e.RawText = raw.RawText
	e.EventDate = raw.EventDate
	e.Summary = raw.Summary
	e.RecordType = raw.RecordType
	e.Channel = raw.Channel
	e.MyFeeling = raw.MyFeeling
	e.TheirReaction = raw.TheirReaction
	e.CreatedAt = raw.CreatedAt
	e.ExtractionStatus = raw.ExtractionStatus
	e.ExtractionError = raw.ExtractionError
	e.ExtractedAt = raw.ExtractedAt
	e.ManuallyEdited = raw.ManuallyEdited
	e.EditedAt = raw.EditedAt
	e.Participants = raw.Participants
	e.Promises = string(raw.Promises)
	return nil
}

// Validate guards the fields the timeline and weekly report sort by. A record
// needs at least one person, either as the primary person_id or via participants.
func (e *Event) Validate() error {
	if err := e.NormalizeParticipants(); err != nil {
		return err
	}
	if strings.TrimSpace(e.RawText) == "" {
		return NewError(ErrInvalidInput, "raw_text is required")
	}
	if e.EventDate == "" {
		return NewError(ErrInvalidInput, "event_date is required")
	}
	if _, err := time.Parse("2006-01-02", e.EventDate); err != nil {
		return NewError(ErrInvalidInput, "event_date must be YYYY-MM-DD")
	}
	return nil
}

// NormalizeParticipants reconciles person_id with the participant list: the
// primary person always appears in it, and a record without a primary person
// adopts its first participant. Keeping both in step is what lets the same
// record be read from every attendee without being stored twice.
func (e *Event) NormalizeParticipants() error {
	seen := map[string]bool{}
	clean := make([]EventParticipant, 0, len(e.Participants)+1)
	for _, p := range e.Participants {
		id := strings.TrimSpace(p.PersonID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		p.PersonID = id
		p.Role = strings.TrimSpace(p.Role)
		clean = append(clean, p)
	}
	if e.PersonID != "" && !seen[e.PersonID] {
		clean = append([]EventParticipant{{PersonID: e.PersonID, Role: PrimaryParticipantRole}}, clean...)
		seen[e.PersonID] = true
	}
	if e.PersonID == "" && len(clean) > 0 {
		e.PersonID = clean[0].PersonID
	}
	// The anchored person always carries a role, even when the caller did not
	// type one, so the attendance list reads the same whether a record came from
	// the API or from the legacy backfill.
	for i := range clean {
		if clean[i].PersonID == e.PersonID && clean[i].Role == "" {
			clean[i].Role = PrimaryParticipantRole
		}
	}
	e.Participants = clean
	if e.PersonID == "" {
		return NewError(ErrInvalidInput, "person_id or participants is required")
	}
	return nil
}

type Trait struct {
	ID             string    `json:"id"`
	PersonID       string    `json:"person_id"`
	TraitKey       string    `json:"trait_key"`
	TraitValue     string    `json:"trait_value"`
	Confidence     float64   `json:"confidence"`
	SourceEventIDs string    `json:"-"`
	Verified       int       `json:"verified"`
	UpdatedAt      time.Time `json:"updated_at"`
	// SourceStale flags a profile note whose evidence was later edited or
	// deleted. The row is kept rather than dropped, because showing "this was
	// derived from a record that has changed" is more honest than silently
	// presenting it as current.
	SourceStale       int    `json:"source_stale"`
	SourceStaleReason string `json:"source_stale_reason"`
}

func (t Trait) MarshalJSON() ([]byte, error) {
	ids := decodeStringList(t.SourceEventIDs)
	return json.Marshal(struct {
		ID                string    `json:"id"`
		PersonID          string    `json:"person_id"`
		TraitKey          string    `json:"trait_key"`
		TraitValue        string    `json:"trait_value"`
		Confidence        float64   `json:"confidence"`
		SourceEventIDs    []string  `json:"source_event_ids"`
		Verified          int       `json:"verified"`
		UpdatedAt         time.Time `json:"updated_at"`
		SourceStale       int       `json:"source_stale"`
		SourceStaleReason string    `json:"source_stale_reason"`
	}{t.ID, t.PersonID, t.TraitKey, t.TraitValue, t.Confidence, ids, t.Verified, t.UpdatedAt,
		t.SourceStale, t.SourceStaleReason})
}

func (t *Trait) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID                string          `json:"id"`
		PersonID          string          `json:"person_id"`
		TraitKey          string          `json:"trait_key"`
		TraitValue        string          `json:"trait_value"`
		Confidence        float64         `json:"confidence"`
		SourceEventIDs    json.RawMessage `json:"source_event_ids"`
		Verified          int             `json:"verified"`
		UpdatedAt         time.Time       `json:"updated_at"`
		SourceStale       int             `json:"source_stale"`
		SourceStaleReason string          `json:"source_stale_reason"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.ID = raw.ID
	t.PersonID = raw.PersonID
	t.TraitKey = raw.TraitKey
	t.TraitValue = raw.TraitValue
	t.Confidence = raw.Confidence
	t.SourceEventIDs = string(raw.SourceEventIDs)
	t.Verified = raw.Verified
	t.UpdatedAt = raw.UpdatedAt
	t.SourceStale = raw.SourceStale
	t.SourceStaleReason = raw.SourceStaleReason
	return nil
}

// decodeStringList tolerates the comma-separated form written by older builds.
func decodeStringList(s string) []string {
	out := []string{}
	if s == "" {
		return out
	}
	if err := json.Unmarshal([]byte(s), &out); err == nil {
		return out
	}
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// EncodeIDList renders an ID list in the JSON array form the columns store.
func EncodeIDList(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(data)
}

type EventExtraction struct {
	Summary       string         `json:"summary"`
	MyFeeling     string         `json:"my_feeling"`
	TheirReaction string         `json:"their_reaction"`
	Promises      []EventPromise `json:"promises"`
}

// IngestReport describes what the AI pipeline managed to do for one record.
type IngestReport struct {
	Extracted     bool     `json:"extracted"`
	Vectorized    bool     `json:"vectorized"`
	TraitsUpdated int      `json:"traits_updated"`
	Warnings      []string `json:"warnings"`
	// Async is true when extraction was queued instead of run inline. The
	// record is stored and visible now; the summary, index and profile refresh
	// land later. Poll the record or GET /api/tasks to observe progress.
	Async bool `json:"async"`
}

type ReindexResult struct {
	EventsIndexed int `json:"events_indexed"`
	TraitsIndexed int `json:"traits_indexed"`
	Failed        int `json:"failed"`
}

type APIResponse struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data"`
	Error *APIError   `json:"error"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
