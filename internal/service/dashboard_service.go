package service

import (
	"database/sql"
	"fmt"
	"time"
)

// PersonRef is a compact person projection for the dashboard: who, how they are
// classified, where they belong and when the referenced event happened.
type PersonRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Relation  string `json:"relation,omitempty"`
	OrgName   string `json:"org_name,omitempty"`
	Gender    string `json:"gender,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// EventRef is the one-record summary for "latest activity".
type EventRef struct {
	ID         string `json:"id"`
	PersonName string `json:"person_name,omitempty"`
	EventDate  string `json:"event_date"`
	Summary    string `json:"summary,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// DashboardStats is the landing-page aggregate. Every number is a cheap count
// over a personal-scale table, and the three "latest" slots answer the first
// questions a returning user asks: who did I add, whose profile just changed,
// and what was the last thing I wrote down.
type DashboardStats struct {
	PersonsTotal       int         `json:"persons_total"`
	OrgsTotal          int         `json:"orgs_total"`
	EventsTotal        int         `json:"events_total"`
	TraitsTotal        int         `json:"traits_total"`
	RelationshipsTotal int         `json:"relationships_total"`
	OpenFollowUps      int         `json:"open_follow_ups"`
	OverdueFollowUps   int         `json:"overdue_follow_ups"`
	LatestPerson       *PersonRef  `json:"latest_person"`
	LatestTraitPerson  *PersonRef  `json:"latest_trait_person"`
	LatestEvent        *EventRef   `json:"latest_event"`
	RecentPersons      []PersonRef `json:"recent_persons"`
}

// DashboardService answers the aggregate queries the landing page renders. It is
// read-only and cheap by design: one round of COUNTs plus three bounded lookups.
type DashboardService struct {
	db *sql.DB
}

func NewDashboardService(db *sql.DB) *DashboardService {
	return &DashboardService{db: db}
}

func (s *DashboardService) Stats() (*DashboardStats, error) {
	stats := &DashboardStats{RecentPersons: []PersonRef{}}

	queries := []struct {
		query string
		dest  *int
	}{
		{`SELECT COUNT(*) FROM persons`, &stats.PersonsTotal},
		{`SELECT COUNT(*) FROM organizations WHERE archived_at IS NULL`, &stats.OrgsTotal},
		{`SELECT COUNT(*) FROM events`, &stats.EventsTotal},
		{`SELECT COUNT(*) FROM traits`, &stats.TraitsTotal},
		{`SELECT COUNT(*) FROM person_relationships`, &stats.RelationshipsTotal},
		{`SELECT COUNT(*) FROM follow_ups WHERE status IN ('pending', 'waiting')`, &stats.OpenFollowUps},
	}
	for _, q := range queries {
		if err := s.db.QueryRow(q.query).Scan(q.dest); err != nil {
			return nil, fmt.Errorf("dashboard count: %w", err)
		}
	}

	// due_date is a YYYY-MM-DD string, so a plain string comparison against
	// today works without parsing on either side.
	today := time.Now().Format("2006-01-02")
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM follow_ups WHERE status IN ('pending', 'waiting')
			AND due_date IS NOT NULL AND due_date < ?`, today,
	).Scan(&stats.OverdueFollowUps); err != nil {
		return nil, fmt.Errorf("dashboard overdue count: %w", err)
	}

	latestPerson, err := scanPersonRef(s.db.QueryRow(`
		SELECT p.id, p.name, COALESCE(p.relation, ''), COALESCE(o.name, ''),
		       COALESCE(p.gender, ''), p.created_at, p.updated_at
		FROM persons p LEFT JOIN organizations o ON o.id = p.org_id
		ORDER BY p.created_at DESC, p.id DESC LIMIT 1`))
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("latest person: %w", err)
	}
	stats.LatestPerson = latestPerson

	latestTrait, err := scanPersonRef(s.db.QueryRow(`
		SELECT p.id, p.name, COALESCE(p.relation, ''), COALESCE(o.name, ''),
		       COALESCE(p.gender, ''), NULL, MAX(t.updated_at)
		FROM traits t
		JOIN persons p ON p.id = t.person_id
		LEFT JOIN organizations o ON o.id = p.org_id
		GROUP BY p.id
		ORDER BY MAX(t.updated_at) DESC, p.id DESC LIMIT 1`))
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("latest trait person: %w", err)
	}
	stats.LatestTraitPerson = latestTrait

	latestEvent, err := scanEventRef(s.db.QueryRow(`
		SELECT e.id, COALESCE(p.name, ''), e.event_date, COALESCE(e.summary, ''), e.created_at
		FROM events e LEFT JOIN persons p ON p.id = e.person_id
		ORDER BY e.created_at DESC, e.id DESC LIMIT 1`))
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("latest event: %w", err)
	}
	stats.LatestEvent = latestEvent

	rows, err := s.db.Query(`
		SELECT p.id, p.name, COALESCE(p.relation, ''), COALESCE(o.name, ''),
		       COALESCE(p.gender, ''), p.created_at, p.updated_at
		FROM persons p LEFT JOIN organizations o ON o.id = p.org_id
		ORDER BY p.created_at DESC, p.id DESC LIMIT 5`)
	if err != nil {
		return nil, fmt.Errorf("recent persons: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		ref, err := scanPersonRef(rows)
		if err != nil {
			return nil, fmt.Errorf("scan recent person: %w", err)
		}
		stats.RecentPersons = append(stats.RecentPersons, *ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}

type personRefScanner interface {
	Scan(dest ...any) error
}

func scanPersonRef(row personRefScanner) (*PersonRef, error) {
	ref := &PersonRef{}
	var created, updated sql.NullString
	if err := row.Scan(&ref.ID, &ref.Name, &ref.Relation, &ref.OrgName, &ref.Gender, &created, &updated); err != nil {
		return nil, err
	}
	ref.CreatedAt = created.String
	ref.UpdatedAt = updated.String
	return ref, nil
}

func scanEventRef(row personRefScanner) (*EventRef, error) {
	ref := &EventRef{}
	var created sql.NullString
	if err := row.Scan(&ref.ID, &ref.PersonName, &ref.EventDate, &ref.Summary, &created); err != nil {
		return nil, err
	}
	ref.CreatedAt = created.String
	return ref, nil
}