package models

import "time"

// PrimaryParticipantRole marks the person a record is anchored to in
// events.person_id. Other participants carry whatever role the user typed.
const PrimaryParticipantRole = "primary"

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
