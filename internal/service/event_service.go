package service

import (
	"fmt"
	"relationship/internal/models"
	"relationship/internal/repository"
)

type EventService struct {
	repo *repository.EventRepo
	vec  *repository.VecRepo
}

func NewEventService(repo *repository.EventRepo, vec *repository.VecRepo) *EventService {
	return &EventService{repo: repo, vec: vec}
}

// Writes go through AIService.IngestEvent so a record never lands in the timeline
// without its extraction and index attempt.

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
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	return s.vec.DeleteBySourceID(id)
}
