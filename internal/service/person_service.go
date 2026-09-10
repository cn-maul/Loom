package service

import (
	"fmt"
	"relationship/internal/models"
	"relationship/internal/repository"
)

type PersonService struct {
	repo *repository.PersonRepo
}

func NewPersonService(repo *repository.PersonRepo) *PersonService {
	return &PersonService{repo: repo}
}

func (s *PersonService) Create(person *models.Person) error {
	if person.Name == "" {
		return fmt.Errorf("name is required")
	}
	if person.Importance == 0 {
		person.Importance = 3
	}
	return s.repo.Create(person)
}

func (s *PersonService) GetByID(id string) (*models.Person, error) {
	return s.repo.GetByID(id)
}

func (s *PersonService) List() ([]*models.Person, error) {
	return s.repo.List()
}

func (s *PersonService) Update(person *models.Person) error {
	if person.ID == "" {
		return fmt.Errorf("id is required")
	}
	return s.repo.Update(person)
}

func (s *PersonService) Delete(id string) error {
	return s.repo.Delete(id)
}