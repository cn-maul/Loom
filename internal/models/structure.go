package models

import (
	"strings"
	"time"
)

// PrimaryParticipantRole marks the person a record is anchored to in
// events.person_id. Other participants carry whatever role the user typed.
const PrimaryParticipantRole = "primary"

// Relationship directions. A directed edge reads from → to ("张总 是 我 的上级"),
// an undirected one is symmetric (同事). Direction is explicit so the graph never
// has to guess and the old free-text `relation` column is never used to invent an
// edge.
const (
	DirectionDirected   = "directed"
	DirectionUndirected = "undirected"
)

// Relationship is a structured, evidence-backed link between two people.
type Relationship struct {
	ID            string    `json:"id"`
	FromPersonID  string    `json:"from_person_id"`
	ToPersonID    string    `json:"to_person_id"`
	RelationType  string    `json:"relation_type"`
	Direction     string    `json:"direction"`
	StartDate     string    `json:"start_date"`
	EndDate       string    `json:"end_date"`
	SourceEventID string    `json:"source_event_id"`
	Confirmed     int       `json:"confirmed"`
	Notes         string    `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// RelationshipLink carries the endpoint names so the graph and lists do not need
// a second lookup per edge.
type RelationshipLink struct {
	Relationship
	FromPersonName string `json:"from_person_name"`
	ToPersonName   string `json:"to_person_name"`
}

func (r *Relationship) Validate() error {
	if strings.TrimSpace(r.FromPersonID) == "" || strings.TrimSpace(r.ToPersonID) == "" {
		return NewError(ErrInvalidInput, "from_person_id and to_person_id are required")
	}
	if r.FromPersonID == r.ToPersonID {
		return NewError(ErrInvalidInput, "a person cannot be related to themselves")
	}
	if strings.TrimSpace(r.RelationType) == "" {
		return NewError(ErrInvalidInput, "relation_type is required")
	}
	if r.Direction == "" {
		r.Direction = DirectionDirected
	}
	if r.Direction != DirectionDirected && r.Direction != DirectionUndirected {
		return NewError(ErrInvalidInput, "direction must be %s or %s", DirectionDirected, DirectionUndirected)
	}
	if r.Confirmed != 0 && r.Confirmed != 1 {
		return NewError(ErrInvalidInput, "confirmed must be 0 or 1")
	}
	return validateDateWindow("start_date", r.StartDate, "end_date", r.EndDate)
}

// RelationshipFilter narrows the graph query. Active is a tri-state: nil means
// either, true means still in force (no end_date), false means ended.
type RelationshipFilter struct {
	PersonID  string
	Type      string
	Confirmed *int
	Active    *bool
	Query     string
	Limit     int
	Offset    int
}

// OrgPosition is one stint of one person at one organisation. end_date NULL means
// the posting is current, so the same table holds both the current member list and
// the history.
type OrgPosition struct {
	ID        string    `json:"id"`
	PersonID  string    `json:"person_id"`
	OrgID     string    `json:"org_id"`
	Role      string    `json:"role"`
	StartDate string    `json:"start_date"`
	EndDate   string    `json:"end_date"`
	Source    string    `json:"source"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgPositionLink adds the display names for the person and the organisation.
type OrgPositionLink struct {
	OrgPosition
	PersonName string `json:"person_name"`
	OrgName    string `json:"org_name"`
}

func (p *OrgPosition) Validate() error {
	if strings.TrimSpace(p.PersonID) == "" {
		return NewError(ErrInvalidInput, "person_id is required")
	}
	if strings.TrimSpace(p.OrgID) == "" {
		return NewError(ErrInvalidInput, "org_id is required")
	}
	return validateDateWindow("start_date", p.StartDate, "end_date", p.EndDate)
}

// EventParticipant is one attendee of a record. The record itself is stored once;
// this is what makes it visible from every participant.
type EventParticipant struct {
	EventID    string `json:"event_id,omitempty"`
	PersonID   string `json:"person_id"`
	PersonName string `json:"person_name,omitempty"`
	Role       string `json:"role"`
}

// EventFilter narrows the record list query across primary person and
// participants. Status filters on the extraction state, which is how the UI
// finds the records that need a retry. OrgID filters by the organisation of any
// attendee (primary person or participant), with the sentinel "none" meaning
// people who belong to no organisation.
type EventFilter struct {
	PersonID string
	OrgID    string
	From     string
	To       string
	Query    string
	Status   string
	Limit    int
	Offset   int
}

// CoAttendance is a derived "shared experience" pair: two people who appear on
// the same record. It is never stored as an edge, because attending the same
// meeting does not mean two people are friends, colleagues or anything else.
type CoAttendance struct {
	PersonAID    string `json:"person_a_id"`
	PersonBID    string `json:"person_b_id"`
	PersonAName  string `json:"person_a_name"`
	PersonBName  string `json:"person_b_name"`
	SharedEvents int    `json:"shared_events"`
}

// validateDateWindow checks an optional start/end pair, both as YYYY-MM-DD and in
// order. Empty values mean "unknown", which the schema stores as NULL rather than
// a fabricated date.
func validateDateWindow(startField, start, endField, end string) error {
	for _, field := range []struct{ name, value string }{
		{startField, start},
		{endField, end},
	} {
		if field.value == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", field.value); err != nil {
			return NewError(ErrInvalidInput, "%s must be YYYY-MM-DD", field.name)
		}
	}
	if start != "" && end != "" && end < start {
		return NewError(ErrInvalidInput, "%s must not be before %s", endField, startField)
	}
	return nil
}

// ValidateDateRange checks an optional from/to query window, inclusive on both ends.
func ValidateDateRange(from, to string) error {
	return validateDateWindow("from", from, "to", to)
}
