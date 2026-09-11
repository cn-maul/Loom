package models

import (
	"encoding/json"
	"fmt"
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
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Person struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Relation   string    `json:"relation"`
	Importance int       `json:"importance"`
	Notes      string    `json:"notes"`
	OrgID      string    `json:"org_id"`
	Position   string    `json:"position"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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

type FollowUp struct {
	ID          string    `json:"id"`
	PersonID    string    `json:"person_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	DueDate     string    `json:"due_date"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

func (f *FollowUp) Validate() error {
	if f.PersonID == "" || strings.TrimSpace(f.Title) == "" {
		return fmt.Errorf("person_id and title are required")
	}
	if f.Status == "" {
		f.Status = "pending"
	}
	for _, s := range []string{"pending", "waiting", "completed", "cancelled"} {
		if f.Status == s {
			return nil
		}
	}
	return fmt.Errorf("invalid status")
}

type Event struct {
	ID            string    `json:"id"`
	PersonID      string    `json:"person_id"`
	RawText       string    `json:"raw_text"`
	EventDate     string    `json:"event_date"`
	Summary       string    `json:"summary"`
	MyFeeling     string    `json:"my_feeling"`
	TheirReaction string    `json:"their_reaction"`
	Promises      string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
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
	return json.Marshal(struct {
		ID            string         `json:"id"`
		PersonID      string         `json:"person_id"`
		RawText       string         `json:"raw_text"`
		EventDate     string         `json:"event_date"`
		Summary       string         `json:"summary"`
		MyFeeling     string         `json:"my_feeling"`
		TheirReaction string         `json:"their_reaction"`
		Promises      []EventPromise `json:"promises"`
		CreatedAt     time.Time      `json:"created_at"`
	}{e.ID, e.PersonID, e.RawText, e.EventDate, e.Summary, e.MyFeeling, e.TheirReaction, promises, e.CreatedAt})
}

// UnmarshalJSON accepts promises both as an array and as the string form.
func (e *Event) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID            string          `json:"id"`
		PersonID      string          `json:"person_id"`
		RawText       string          `json:"raw_text"`
		EventDate     string          `json:"event_date"`
		Summary       string          `json:"summary"`
		MyFeeling     string          `json:"my_feeling"`
		TheirReaction string          `json:"their_reaction"`
		Promises      json.RawMessage `json:"promises"`
		CreatedAt     time.Time       `json:"created_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.ID = raw.ID
	e.PersonID = raw.PersonID
	e.RawText = raw.RawText
	e.EventDate = raw.EventDate
	e.Summary = raw.Summary
	e.MyFeeling = raw.MyFeeling
	e.TheirReaction = raw.TheirReaction
	e.CreatedAt = raw.CreatedAt
	e.Promises = string(raw.Promises)
	return nil
}

// Validate guards the fields the timeline and weekly report sort by.
func (e *Event) Validate() error {
	if e.PersonID == "" {
		return fmt.Errorf("person_id is required")
	}
	if strings.TrimSpace(e.RawText) == "" {
		return fmt.Errorf("raw_text is required")
	}
	if e.EventDate == "" {
		return fmt.Errorf("event_date is required")
	}
	if _, err := time.Parse("2006-01-02", e.EventDate); err != nil {
		return fmt.Errorf("event_date must be YYYY-MM-DD")
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
}

func (t Trait) MarshalJSON() ([]byte, error) {
	ids := decodeStringList(t.SourceEventIDs)
	return json.Marshal(struct {
		ID             string    `json:"id"`
		PersonID       string    `json:"person_id"`
		TraitKey       string    `json:"trait_key"`
		TraitValue     string    `json:"trait_value"`
		Confidence     float64   `json:"confidence"`
		SourceEventIDs []string  `json:"source_event_ids"`
		Verified       int       `json:"verified"`
		UpdatedAt      time.Time `json:"updated_at"`
	}{t.ID, t.PersonID, t.TraitKey, t.TraitValue, t.Confidence, ids, t.Verified, t.UpdatedAt})
}

func (t *Trait) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID             string          `json:"id"`
		PersonID       string          `json:"person_id"`
		TraitKey       string          `json:"trait_key"`
		TraitValue     string          `json:"trait_value"`
		Confidence     float64         `json:"confidence"`
		SourceEventIDs json.RawMessage `json:"source_event_ids"`
		Verified       int             `json:"verified"`
		UpdatedAt      time.Time       `json:"updated_at"`
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

type AdviceRequest struct {
	PersonID string `json:"person_id"`
	Question string `json:"question"`
}

type AdviceResponse struct {
	Situation        string     `json:"situation"`
	OtherPerspective string     `json:"other_perspective"`
	Risks            []string   `json:"risks"`
	Strategies       []Strategy `json:"strategies"`
	EvidenceEventIDs []string   `json:"evidence_event_ids"`
	FollowUp         string     `json:"follow_up"`
	// VectorUsed reports whether the semantic search branch contributed context.
	VectorUsed bool `json:"vector_used"`
}

type Strategy struct {
	Name   string `json:"name"`
	Script string `json:"script"`
	Pros   string `json:"pros"`
	Cons   string `json:"cons"`
}

// IngestReport describes what the AI pipeline managed to do for one record.
type IngestReport struct {
	Extracted     bool     `json:"extracted"`
	Vectorized    bool     `json:"vectorized"`
	TraitsUpdated int      `json:"traits_updated"`
	Warnings      []string `json:"warnings"`
}

type WeeklyReport struct {
	Start       string        `json:"start"`
	End         string        `json:"end"`
	EventCount  int           `json:"event_count"`
	Summary     string        `json:"summary"`
	Persons     []WeeklySlice `json:"persons"`
	GeneratedBy string        `json:"generated_by"`
}

type WeeklySlice struct {
	PersonID   string   `json:"person_id"`
	PersonName string   `json:"person_name"`
	Events     []string `json:"events"`
	Promises   []string `json:"promises"`
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
