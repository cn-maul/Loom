package service

import (
	"fmt"
	"github.com/google/uuid"
	"relationship/internal/models"
	"relationship/internal/repository"
	"time"
)

type FollowUpService struct {
	repo    *repository.FollowUpRepo
	persons *PersonService
}

func NewFollowUpService(repo *repository.FollowUpRepo, persons *PersonService) *FollowUpService {
	return &FollowUpService{repo: repo, persons: persons}
}
func (s *FollowUpService) Create(f *models.FollowUp) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if _, err := s.persons.GetByID(f.PersonID); err != nil {
		return fmt.Errorf("person not found: %w", err)
	}
	f.ID = uuid.New().String()
	return s.repo.Create(f)
}
func (s *FollowUpService) Get(id string) (*models.FollowUp, error) { return s.repo.Get(id) }
func (s *FollowUpService) List(personID, status, from, to string) ([]*models.FollowUp, error) {
	return s.repo.List(personID, status, from, to)
}
func (s *FollowUpService) Update(f *models.FollowUp) error {
	if f.ID == "" {
		return fmt.Errorf("id is required")
	}
	if err := f.Validate(); err != nil {
		return err
	}
	return s.repo.Update(f)
}
func (s *FollowUpService) Postpone(id, dueDate string) error {
	if _, err := time.Parse("2006-01-02", dueDate); err != nil {
		return fmt.Errorf("due_date must be YYYY-MM-DD")
	}
	return s.repo.Postpone(id, dueDate)
}

func (s *FollowUpService) Action(id, status string) error {
	if status != "completed" && status != "waiting" && status != "cancelled" {
		return fmt.Errorf("invalid status")
	}
	return s.repo.Action(id, status)
}
