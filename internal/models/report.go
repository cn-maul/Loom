package models

import (
	"strings"
	"time"
)

// Report generation outcomes. A report whose narrative call failed is still
// stored, with the structured aggregation intact: the wording is what broke,
// not the facts, and throwing the report away would lose the facts with it.
const (
	ReportSucceeded = "succeeded"
	ReportFailed    = "failed"
)

// The buckets a report sorts open work into. They are disjoint and together
// cover every unclosed item, so nothing silently drops off a report and the
// reader never has to reconcile two lists that overlap.
const (
	ReportOverdue     = "overdue"
	ReportDueSoon     = "due_soon"
	ReportWaiting     = "waiting"
	ReportCarriedOver = "carried_over"
	ReportUpcoming    = "upcoming"
	ReportCompleted   = "completed"
)

// Kinds of change the report can cite from the relationship graph.
const (
	ReportRelationshipStarted = "relationship_started"
	ReportRelationshipEnded   = "relationship_ended"
	ReportPositionStarted     = "position_started"
	ReportPositionEnded       = "position_ended"
)

// MaxReportSpanDays bounds one report. Past a year this stops being a report
// and becomes an export, which is a different feature with different rules.
const MaxReportSpanDays = 366

// DateLayout is the calendar date format every user-facing date uses.
const DateLayout = "2006-01-02"

// ReportRequest asks for one period. Dates are inclusive and local to the user.
type ReportRequest struct {
	PersonID string `json:"person_id"`
	Start    string `json:"start"`
	End      string `json:"end"`
	// WeekOf, when set, replaces Start/End with the natural week (Monday to
	// Sunday) containing that date. One definition of "last week" belongs in
	// the backend, not in each caller that wants one.
	WeekOf string `json:"week_of"`
}

// Resolve validates the request and writes the resolved window back onto it, so
// a persisted snapshot records the period that was actually reported on rather
// than whatever the caller happened to type.
func (r *ReportRequest) Resolve(now time.Time) error {
	r.PersonID = strings.TrimSpace(r.PersonID)
	r.Start = strings.TrimSpace(r.Start)
	r.End = strings.TrimSpace(r.End)
	r.WeekOf = strings.TrimSpace(r.WeekOf)

	switch {
	case r.WeekOf != "":
		anchor, err := time.Parse(DateLayout, r.WeekOf)
		if err != nil {
			return NewError(ErrInvalidInput, "week_of must be YYYY-MM-DD")
		}
		// Go counts weekdays from Sunday; the natural week here starts Monday.
		monday := anchor.AddDate(0, 0, -((int(anchor.Weekday()) + 6) % 7))
		r.Start = monday.Format(DateLayout)
		r.End = monday.AddDate(0, 0, 6).Format(DateLayout)
	case r.Start == "" && r.End == "":
		// The default is the seven days ending today, which is what "this week"
		// means to someone opening the page on a Friday afternoon.
		r.End = now.Format(DateLayout)
		r.Start = now.AddDate(0, 0, -6).Format(DateLayout)
	case r.Start == "" || r.End == "":
		return NewError(ErrInvalidInput, "start and end must be given together")
	}

	start, err := time.Parse(DateLayout, r.Start)
	if err != nil {
		return NewError(ErrInvalidInput, "start must be YYYY-MM-DD")
	}
	end, err := time.Parse(DateLayout, r.End)
	if err != nil {
		return NewError(ErrInvalidInput, "end must be YYYY-MM-DD")
	}
	if end.Before(start) {
		return NewError(ErrInvalidInput, "end must not be before start")
	}
	if span := int(end.Sub(start).Hours()/24) + 1; span > MaxReportSpanDays {
		return NewError(ErrInvalidInput, "a report may cover at most %d days, got %d", MaxReportSpanDays, span)
	}
	return nil
}

// DayCount is the inclusive length of the resolved window.
func (r *ReportRequest) DayCount() int {
	start, err := time.Parse(DateLayout, r.Start)
	if err != nil {
		return 0
	}
	end, err := time.Parse(DateLayout, r.End)
	if err != nil {
		return 0
	}
	return int(end.Sub(start).Hours()/24) + 1
}

// ReportFollowUpRef is one follow-up as a report cites it: an id that opens the
// item, plus just enough text to read it in place. Reports used to flatten
// items into sentences, which meant a reader could not act on what they read.
type ReportFollowUpRef struct {
	ID          string `json:"id"`
	PersonID    string `json:"person_id"`
	PersonName  string `json:"person_name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Owner       string `json:"owner"`
	DueDate     string `json:"due_date"`
	DueText     string `json:"due_text"`
	// DaysOverdue is only meaningful in the overdue bucket; it is 0 elsewhere.
	DaysOverdue int `json:"days_overdue"`
	// SourceEventID / SourceAdviceID point back at what the item came from, so
	// "why do I have this to do" is one click away.
	SourceEventID  string `json:"source_event_id"`
	SourceAdviceID string `json:"source_advice_id"`
}

// ReportEventRef is one record inside the period.
type ReportEventRef struct {
	ID       string `json:"id"`
	PersonID string `json:"person_id"`
	// PersonName is the record's own anchor person. A report scoped to someone
	// who merely attended the record still names the record by its owner.
	PersonName       string `json:"person_name"`
	EventDate        string `json:"event_date"`
	Summary          string `json:"summary"`
	RecordType       string `json:"record_type"`
	ExtractionStatus string `json:"extraction_status"`
}

// ReportPromise is a commitment made inside the period, tied to the record it
// came out of rather than reproduced as a floating sentence.
type ReportPromise struct {
	EventID    string `json:"event_id"`
	PersonID   string `json:"person_id"`
	PersonName string `json:"person_name"`
	EventDate  string `json:"event_date"`
	Who        string `json:"who"`
	What       string `json:"what"`
	Deadline   string `json:"deadline"`
}

// ReportChangeRef is one change to the relationship graph or an organisation
// posting inside the period.
type ReportChangeRef struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	PersonID   string `json:"person_id"`
	PersonName string `json:"person_name"`
	// CounterpartID is the other end of the change: the other person for a
	// relationship edge, the organisation for a posting.
	CounterpartID   string `json:"counterpart_id"`
	CounterpartName string `json:"counterpart_name"`
	Description     string `json:"description"`
	Date            string `json:"date"`
}

// ReportPersonSummary groups the period per person, which is what makes "who
// needs attention" readable without scrolling the whole timeline.
type ReportPersonSummary struct {
	PersonID      string `json:"person_id"`
	PersonName    string `json:"person_name"`
	EventCount    int    `json:"event_count"`
	LastEventDate string `json:"last_event_date"`
	OpenFollowUps int    `json:"open_follow_ups"`
}

// Report is one period's report. The structured lists are produced before the
// narrative is attempted, because they are the part that must survive a broken
// model: only the wording is allowed to degrade.
type Report struct {
	ID         string `json:"id"`
	PersonID   string `json:"person_id"`
	PersonName string `json:"person_name"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Days       int    `json:"days"`
	// Status is succeeded or failed. GeneratedBy names the model that wrote the
	// narrative, or "local" when the aggregation was rendered as the text.
	Status        string    `json:"status"`
	GeneratedBy   string    `json:"generated_by"`
	FailureReason string    `json:"failure_reason,omitempty"`
	GeneratedAt   time.Time `json:"generated_at"`
	Summary       string    `json:"summary"`

	// 1. Late work.
	Overdue []ReportFollowUpRef `json:"overdue"`
	// 2. Due inside the period, and work waiting on the other side.
	DueSoon []ReportFollowUpRef `json:"due_soon"`
	Waiting []ReportFollowUpRef `json:"waiting"`
	// 3. Still open from before the period began: the part that is easy to lose.
	CarriedOver []ReportFollowUpRef `json:"carried_over"`
	// 4. Not yet due, or not yet scheduled: next period's plan.
	Upcoming []ReportFollowUpRef `json:"upcoming"`
	// Closed inside the period, so the report shows outcomes and not only debt.
	Completed []ReportFollowUpRef `json:"completed"`
	// 5. What changed on the graph, and what was recorded.
	Changes  []ReportChangeRef     `json:"changes"`
	Events   []ReportEventRef      `json:"events"`
	Promises []ReportPromise       `json:"promises"`
	Persons  []ReportPersonSummary `json:"persons"`

	EventCount   int `json:"event_count"`
	PromiseCount int `json:"promise_count"`
	OpenCount    int `json:"open_count"`
}

// HasFindings reports whether the period holds anything worth narrating. An
// empty period is answered locally instead of asking a model to write prose
// about nothing, which is how a report invents a busy week.
func (r *Report) HasFindings() bool {
	return r.EventCount > 0 || r.OpenCount > 0 || len(r.Changes) > 0 ||
		len(r.Completed) > 0 || len(r.Promises) > 0
}

// ReportSnapshot is a history row: the window, when it was made and the
// headline. The sections are fetched by id, so listing a year of reports does
// not ship a year of payloads.
type ReportSnapshot struct {
	ID          string `json:"id"`
	PersonID    string `json:"person_id"`
	PersonName  string `json:"person_name"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Status      string `json:"status"`
	GeneratedBy string `json:"generated_by"`
	// FailureReason lets the history mark a degraded snapshot — the structured
	// aggregation is there, only the wording is missing — without loading it.
	FailureReason string    `json:"failure_reason,omitempty"`
	Summary       string    `json:"summary"`
	EventCount    int       `json:"event_count"`
	GeneratedAt   time.Time `json:"generated_at"`
}
