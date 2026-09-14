package service

import (
	"errors"

	"github.com/google/uuid"

	"relationship/internal/models"
	"relationship/internal/repository"
)

// List windows are shared by the paged endpoints.
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// normalizePage clamps the requested window so a caller cannot ask for the whole
// table in one request.
func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

type RelationshipService struct {
	repo    *repository.RelationshipRepo
	persons *PersonService
	events  *EventService
}

func NewRelationshipService(repo *repository.RelationshipRepo, persons *PersonService, events *EventService) *RelationshipService {
	return &RelationshipService{repo: repo, persons: persons, events: events}
}

func (s *RelationshipService) Create(rel *models.Relationship) (*models.RelationshipLink, error) {
	if err := rel.Validate(); err != nil {
		return nil, err
	}
	if err := s.ensureEndpoints(rel); err != nil {
		return nil, err
	}
	rel.ID = uuid.New().String()
	if err := s.repo.Create(rel); err != nil {
		return nil, err
	}
	return s.repo.GetByID(rel.ID)
}

func (s *RelationshipService) List(filter models.RelationshipFilter) ([]*models.RelationshipLink, error) {
	filter.Limit, filter.Offset = normalizePage(filter.Limit, filter.Offset)
	return s.repo.List(filter)
}

// Update is also how a relationship is closed: sending an end_date keeps the edge
// with its history instead of deleting it.
func (s *RelationshipService) Update(rel *models.Relationship) (*models.RelationshipLink, error) {
	if rel.ID == "" {
		return nil, models.NewError(models.ErrInvalidInput, "id is required")
	}
	if _, err := s.repo.GetByID(rel.ID); err != nil {
		return nil, err
	}
	if err := rel.Validate(); err != nil {
		return nil, err
	}
	if err := s.ensureEndpoints(rel); err != nil {
		return nil, err
	}
	if err := s.repo.Update(rel); err != nil {
		return nil, err
	}
	return s.repo.GetByID(rel.ID)
}

func (s *RelationshipService) Delete(id string) error {
	return s.repo.Delete(id)
}

// ListTypes backs the type picker on the graph page.
func (s *RelationshipService) ListTypes() ([]string, error) {
	return s.repo.ListTypes()
}

func (s *RelationshipService) ensureEndpoints(rel *models.Relationship) error {
	for _, id := range []string{rel.FromPersonID, rel.ToPersonID} {
		if err := s.ensurePerson(id); err != nil {
			return err
		}
	}
	if rel.SourceEventID == "" {
		return nil
	}
	// An edge may cite the record it was inferred from, but a dangling citation
	// would make the graph unverifiable.
	if _, err := s.events.GetByID(rel.SourceEventID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return models.NewError(models.ErrNotFound, "source event %s not found", rel.SourceEventID)
		}
		return err
	}
	return nil
}

func (s *RelationshipService) ensurePerson(id string) error {
	if _, err := s.persons.GetByID(id); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return models.NewError(models.ErrPersonNotFound, "person %s not found", id)
		}
		return err
	}
	return nil
}
