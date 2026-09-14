package service

import (
	"strings"

	"relationship/internal/models"
	"relationship/internal/repository"
)

type OrganizationService struct {
	repo *repository.OrganizationRepo
}

func NewOrganizationService(repo *repository.OrganizationRepo) *OrganizationService {
	return &OrganizationService{repo: repo}
}

func (s *OrganizationService) Create(org *models.Organization) error {
	if err := org.Validate(); err != nil {
		return err
	}
	org.Name = strings.TrimSpace(org.Name)
	return s.repo.Create(org)
}

// GetByID resolves one organisation. There is no HTTP single-read route.
func (s *OrganizationService) GetByID(id string) (*models.Organization, error) {
	return s.repo.GetByID(id)
}

// List returns active organisations by default; the management page asks for
// everything with includeArchived and separates the two states itself.
func (s *OrganizationService) List(includeArchived bool) ([]*models.Organization, error) {
	if includeArchived {
		return s.repo.ListAll()
	}
	return s.repo.List()
}

func (s *OrganizationService) Update(org *models.Organization) error {
	if org.ID == "" {
		return models.NewError(models.ErrInvalidInput, "id is required")
	}
	if err := org.Validate(); err != nil {
		return err
	}
	org.Name = strings.TrimSpace(org.Name)
	return s.repo.Update(org)
}

// Archive hides an organisation from every picker without losing it: the people
// who were attached to it keep their employer. It is the reversible alternative
// to Delete, which detaches them.
func (s *OrganizationService) Archive(id string) error {
	return s.repo.Archive(id)
}

func (s *OrganizationService) Restore(id string) error {
	return s.repo.Restore(id)
}

func (s *OrganizationService) Delete(id string) error {
	return s.repo.Delete(id)
}
