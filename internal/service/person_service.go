package service

import (
	"fmt"
	"relationship/internal/models"
	"relationship/internal/repository"
)

type PersonService struct {
	repo    *repository.PersonRepo
	vec     *repository.VecRepo
	orgRepo *repository.OrganizationRepo
}

func NewPersonService(repo *repository.PersonRepo, vec *repository.VecRepo, orgRepo *repository.OrganizationRepo) *PersonService {
	return &PersonService{repo: repo, vec: vec, orgRepo: orgRepo}
}

func (s *PersonService) Create(person *models.Person) error {
	if person.Name == "" {
		return fmt.Errorf("name is required")
	}
	if person.Importance == 0 {
		person.Importance = 3
	}
	if err := s.ensureOrg(person.OrgID); err != nil {
		return err
	}
	return s.repo.Create(person)
}

func (s *PersonService) GetByID(id string) (*models.Person, error) {
	return s.repo.GetByID(id)
}

func (s *PersonService) List() ([]*models.PersonWithActivity, error) {
	return s.repo.ListWithActivity()
}

func (s *PersonService) Update(person *models.Person) error {
	if person.ID == "" {
		return fmt.Errorf("id is required")
	}
	if err := s.ensureOrg(person.OrgID); err != nil {
		return err
	}
	return s.repo.Update(person)
}

// ensureOrg rejects dangling affiliations instead of relying on the foreign key.
func (s *PersonService) ensureOrg(orgID string) error {
	if orgID == "" {
		return nil
	}
	if _, err := s.orgRepo.GetByID(orgID); err != nil {
		return fmt.Errorf("organization %s does not exist", orgID)
	}
	return nil
}

// Delete drops the person, whose foreign keys cascade to events and traits.
// vec_memory is a virtual table with no foreign key, so it is cleaned separately.
func (s *PersonService) Delete(id string) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	return s.vec.DeleteByPerson(id)
}
