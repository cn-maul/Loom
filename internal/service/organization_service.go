package service

import (
	"fmt"
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
	if org.Name == "" {
		return fmt.Errorf("name is required")
	}
	return s.repo.Create(org)
}

func (s *OrganizationService) GetByID(id string) (*models.Organization, error) {
	return s.repo.GetByID(id)
}

func (s *OrganizationService) List() ([]*models.Organization, error) {
	return s.repo.List()
}

func (s *OrganizationService) Update(org *models.Organization) error {
	if org.ID == "" {
		return fmt.Errorf("id is required")
	}
	if org.Name == "" {
		return fmt.Errorf("name is required")
	}
	return s.repo.Update(org)
}

func (s *OrganizationService) Delete(id string) error {
	return s.repo.Delete(id)
}
