package service

import (
	"errors"

	"github.com/google/uuid"

	"relationship/internal/models"
	"relationship/internal/repository"
)

type PositionService struct {
	repo    *repository.PositionRepo
	persons *PersonService
	orgs    *OrganizationService
}

func NewPositionService(repo *repository.PositionRepo, persons *PersonService, orgs *OrganizationService) *PositionService {
	return &PositionService{repo: repo, persons: persons, orgs: orgs}
}

func (s *PositionService) Create(pos *models.OrgPosition) (*models.OrgPositionLink, error) {
	if err := pos.Validate(); err != nil {
		return nil, err
	}
	if err := s.ensureRefs(pos); err != nil {
		return nil, err
	}
	pos.ID = uuid.New().String()
	if err := s.repo.Create(pos); err != nil {
		return nil, err
	}
	return s.repo.GetByID(pos.ID)
}

func (s *PositionService) GetByID(id string) (*models.OrgPositionLink, error) {
	return s.repo.GetByID(id)
}

func (s *PositionService) ListByPerson(personID string) ([]*models.OrgPositionLink, error) {
	if _, err := s.persons.GetByID(personID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return nil, models.NewError(models.ErrPersonNotFound, "person %s not found", personID)
		}
		return nil, err
	}
	return s.repo.ListByPerson(personID)
}

// ListByOrg returns an organisation's members. The organisation must exist so a
// typo is a 404 rather than an empty list.
func (s *PositionService) ListByOrg(orgID string, currentOnly bool) ([]*models.OrgPositionLink, error) {
	if _, err := s.orgs.GetByID(orgID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return nil, models.NewError(models.ErrOrgNotFound, "organization %s not found", orgID)
		}
		return nil, err
	}
	return s.repo.ListByOrg(orgID, currentOnly)
}

// Update is also how a posting ends: sending an end_date keeps the stint in the
// history instead of removing it.
func (s *PositionService) Update(pos *models.OrgPosition) (*models.OrgPositionLink, error) {
	if pos.ID == "" {
		return nil, models.NewError(models.ErrInvalidInput, "id is required")
	}
	if _, err := s.repo.GetByID(pos.ID); err != nil {
		return nil, err
	}
	if err := pos.Validate(); err != nil {
		return nil, err
	}
	if err := s.ensureRefs(pos); err != nil {
		return nil, err
	}
	if err := s.repo.Update(pos); err != nil {
		return nil, err
	}
	return s.repo.GetByID(pos.ID)
}

func (s *PositionService) Delete(id string) error {
	return s.repo.Delete(id)
}

func (s *PositionService) ensureRefs(pos *models.OrgPosition) error {
	if _, err := s.persons.GetByID(pos.PersonID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return models.NewError(models.ErrPersonNotFound, "person %s not found", pos.PersonID)
		}
		return err
	}
	if _, err := s.orgs.GetByID(pos.OrgID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return models.NewError(models.ErrOrgNotFound, "organization %s not found", pos.OrgID)
		}
		return err
	}
	return nil
}
