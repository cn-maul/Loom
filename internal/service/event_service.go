package service

import (
	"fmt"
	"relationship/internal/models"
	"relationship/internal/repository"
)

type EventService struct {
	repo *repository.EventRepo
}

func NewEventService(repo *repository.EventRepo) *EventService {
	return &EventService{repo: repo}
}

func (s *EventService) Create(event *models.Event) error {
	if event.PersonID == "" {
		return fmt.Errorf("person_id is required")
	}
	if event.RawText == "" {
		return fmt.Errorf("raw_text is required")
	}
	if event.EventDate == "" {
		return fmt.Errorf("event_date is required")
	}
	return s.repo.Create(event)
}

func (s *EventService) GetByID(id string) (*models.Event, error) {
	return s.repo.GetByID(id)
}

func (s *EventService) ListByPerson(personID string, limit int) ([]*models.Event, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListByPerson(personID, limit)
}

func (s *EventService) Update(event *models.Event) error {
	if event.ID == "" {
		return fmt.Errorf("id is required")
	}
	return s.repo.Update(event)
}

func (s *EventService) Delete(id string) error {
	return s.repo.Delete(id)
}