package service

import (
	"relationship/internal/models"
	"relationship/internal/repository"
)

// GraphService assembles the /relationships canvas payload from the structured
// tables. It invents nothing: person-person edges come from person_relationships,
// person-org edges from current postings, and co-attendance stays a separate
// derived list that the UI must never render as a typed relationship.
type GraphService struct {
	persons *repository.PersonRepo
	graph   *repository.GraphRepo
}

func NewGraphService(persons *repository.PersonRepo, graph *repository.GraphRepo) *GraphService {
	return &GraphService{persons: persons, graph: graph}
}

// Build returns the whole graph in one payload. Person nodes reuse the person
// list projection so node metadata (org, activity) matches the home page exactly.
func (s *GraphService) Build() (*models.GraphData, error) {
	persons, _, err := s.persons.ListFiltered(models.PersonFilter{})
	if err != nil {
		return nil, err
	}
	orgs, err := s.graph.ListOrgs()
	if err != nil {
		return nil, err
	}
	relEdges, err := s.graph.ListRelationshipEdges()
	if err != nil {
		return nil, err
	}
	posEdges, err := s.graph.ListPositionEdges()
	if err != nil {
		return nil, err
	}
	coLinks, err := s.graph.ListCoLinks(500)
	if err != nil {
		return nil, err
	}

	nodes := make([]*models.GraphNode, 0, len(persons))
	for _, person := range persons {
		nodes = append(nodes, &models.GraphNode{
			ID:         person.ID,
			Name:       person.Name,
			IsSelf:     person.IsSelf,
			OrgID:      person.OrgID,
			OrgName:    person.OrgName,
			Importance: person.Importance,
			EventCount: person.EventCount,
		})
	}

	edges := make([]*models.GraphEdge, 0, len(relEdges)+len(posEdges))
	edges = append(edges, relEdges...)
	edges = append(edges, posEdges...)

	return &models.GraphData{
		Nodes:        nodes,
		Orgs:         orgs,
		Edges:        edges,
		CoAttendance: coLinks,
	}, nil
}
