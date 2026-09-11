package repository

import (
	"database/sql"
	"fmt"

	"relationship/internal/models"
)

// GraphRepo holds the read-only queries behind GET /api/graph. Each one maps to
// one edge family or node set of the canvas; the service stitches them together.
type GraphRepo struct {
	db *sql.DB
}

func NewGraphRepo(db *sql.DB) *GraphRepo {
	return &GraphRepo{db: db}
}

// ListOrgs returns every active organisation with its current member count, so
// empty organisations can still appear (as small nodes) but archived ones do not.
func (r *GraphRepo) ListOrgs() ([]*models.GraphOrg, error) {
	rows, err := r.db.Query(`
		SELECT o.id, o.name, COALESCE(o.kind, ''),
		       (SELECT COUNT(*) FROM person_org_positions pos
		        WHERE pos.org_id = o.id AND pos.end_date IS NULL) AS members
		FROM organizations o
		WHERE o.archived_at IS NULL
		ORDER BY o.name COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("list graph orgs: %w", err)
	}
	defer rows.Close()

	orgs := []*models.GraphOrg{}
	for rows.Next() {
		org := &models.GraphOrg{}
		var kind sql.NullString
		if err := rows.Scan(&org.ID, &org.Name, &kind, &org.MemberCount); err != nil {
			return nil, fmt.Errorf("scan graph org: %w", err)
		}
		org.Kind = scanNullString(&kind)
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

// ListRelationshipEdges returns every structured person-person edge, ended ones
// included: the canvas greys them out instead of hiding history.
func (r *GraphRepo) ListRelationshipEdges() ([]*models.GraphEdge, error) {
	rows, err := r.db.Query(`
		SELECT id, from_person_id, to_person_id, relation_type, direction,
		       COALESCE(start_date, ''), COALESCE(end_date, ''), confirmed,
		       COALESCE(source_event_id, ''), COALESCE(notes, '')
		FROM person_relationships
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list graph relationship edges: %w", err)
	}
	defer rows.Close()

	edges := []*models.GraphEdge{}
	for rows.Next() {
		edge := &models.GraphEdge{Kind: models.GraphEdgeRelationship}
		if err := rows.Scan(&edge.ID, &edge.From, &edge.To, &edge.Type, &edge.Direction,
			&edge.StartDate, &edge.EndDate, &edge.Confirmed, &edge.SourceEventID, &edge.Notes); err != nil {
			return nil, fmt.Errorf("scan graph relationship edge: %w", err)
		}
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

// ListPositionEdges returns the current postings as person→organisation edges.
// A posting with an end_date is history; history belongs to the organisation
// page, not to the graph's living picture.
func (r *GraphRepo) ListPositionEdges() ([]*models.GraphEdge, error) {
	rows, err := r.db.Query(`
		SELECT pos.id, pos.person_id, pos.org_id, COALESCE(pos.role, ''),
		       COALESCE(pos.start_date, ''), COALESCE(pos.notes, '')
		FROM person_org_positions pos
		WHERE pos.end_date IS NULL
		ORDER BY pos.created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list graph position edges: %w", err)
	}
	defer rows.Close()

	edges := []*models.GraphEdge{}
	for rows.Next() {
		edge := &models.GraphEdge{Kind: models.GraphEdgePosition, Direction: models.DirectionDirected}
		if err := rows.Scan(&edge.ID, &edge.From, &edge.To, &edge.Type, &edge.StartDate, &edge.Notes); err != nil {
			return nil, fmt.Errorf("scan graph position edge: %w", err)
		}
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

// ListCoLinks derives the shared-experience pairs for the canvas. Unlike the
// participants-only co-attendance list, this one unions the anchored person in:
// a record saved with just a primary person still puts that person on the
// canvas, so "我和张总各有一半记录" does not render two isolated islands.
func (r *GraphRepo) ListCoLinks(limit int) ([]*models.GraphCoLink, error) {
	rows, err := r.db.Query(`
		WITH attendees AS (
			SELECT event_id, person_id FROM event_participants
			UNION
			SELECT id, person_id FROM events WHERE person_id IS NOT NULL
		)
		SELECT a.person_id, b.person_id, COALESCE(pa.name, ''), COALESCE(pb.name, ''), COUNT(*) AS shared
		FROM attendees a
		JOIN attendees b ON b.event_id = a.event_id AND b.person_id > a.person_id
		LEFT JOIN persons pa ON pa.id = a.person_id
		LEFT JOIN persons pb ON pb.id = b.person_id
		GROUP BY a.person_id, b.person_id
		ORDER BY shared DESC, a.person_id, b.person_id
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list graph co-links: %w", err)
	}
	defer rows.Close()

	links := []*models.GraphCoLink{}
	for rows.Next() {
		link := &models.GraphCoLink{}
		if err := rows.Scan(&link.A, &link.B, &link.AName, &link.BName, &link.SharedCnt); err != nil {
			return nil, fmt.Errorf("scan graph co-link: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}
