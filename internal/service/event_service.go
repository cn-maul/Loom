package service

import (
	"errors"
	"log"

	"relationship/internal/models"
	"relationship/internal/repository"
)

type EventService struct {
	repo    *repository.EventRepo
	vec     *repository.VecRepo
	persons *PersonService
	traits  *repository.TraitRepo
}

func NewEventService(repo *repository.EventRepo, vec *repository.VecRepo, persons *PersonService, traits *repository.TraitRepo) *EventService {
	return &EventService{repo: repo, vec: vec, persons: persons, traits: traits}
}

// extractionStatuses is the closed set the record pipeline reports.
var extractionStatuses = map[string]bool{
	models.ExtractionPending: true, models.ExtractionSucceeded: true, models.ExtractionFailed: true,
}

// Writes go through AIService.IngestEvent so a record never lands in the timeline
// without its extraction and index attempt.

func (s *EventService) GetByID(id string) (*models.Event, error) {
	return s.repo.GetByID(id)
}

// ListByPerson returns every record the person attended, including records where
// they are a participant rather than the primary person.
func (s *EventService) ListByPerson(personID string, limit int) ([]*models.Event, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	return s.repo.ListByPerson(personID, limit)
}

// List answers the record list query with time range, keyword, extraction state
// and page window, and reports how many records match in total.
func (s *EventService) List(filter models.EventFilter) ([]*models.Event, int, error) {
	if err := models.ValidateDateRange(filter.From, filter.To); err != nil {
		return nil, 0, err
	}
	if filter.Status != "" && !extractionStatuses[filter.Status] {
		return nil, 0, models.NewError(models.ErrInvalidInput, "unknown extraction status %q", filter.Status)
	}
	filter.Limit, filter.Offset = normalizePage(filter.Limit, filter.Offset)
	return s.repo.ListFiltered(filter)
}

// Update applies a hand edit and, when the raw text actually changed, marks the
// profile notes derived from this record as stale: their evidence is no longer
// what it was when the conclusion was drawn.
func (s *EventService) Update(event *models.Event) error {
	if event.ID == "" {
		return models.NewError(models.ErrInvalidInput, "id is required")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	previous, err := s.repo.GetByID(event.ID)
	if err != nil {
		return err
	}
	if err := s.repo.Update(event); err != nil {
		return err
	}
	if previous.RawText != event.RawText {
		s.markTraitsStale(event.ID, "来源记录已修改，画像需要重新计算")
	}
	return nil
}

func (s *EventService) Delete(id string) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	if err := s.vec.DeleteBySourceID(id); err != nil {
		return err
	}
	s.markTraitsStale(id, "来源记录已删除，画像需要重新计算")
	return nil
}

// markTraitsStale is best-effort: failing to annotate a derived profile must not
// undo the edit or the delete the user actually asked for.
func (s *EventService) markTraitsStale(eventID, reason string) {
	if s.traits == nil {
		return
	}
	if _, err := s.traits.MarkSourceStale(eventID, reason); err != nil {
		log.Printf("warning: mark traits stale for event %s: %v", eventID, err)
	}
}

func (s *EventService) ListParticipants(eventID string) ([]models.EventParticipant, error) {
	if _, err := s.repo.GetByID(eventID); err != nil {
		return nil, err
	}
	return s.repo.ListParticipants(eventID)
}

// SetParticipants replaces a record's attendance list. Every participant must be
// a known person, so a mistyped id fails loudly instead of creating an invisible
// half-filled record.
func (s *EventService) SetParticipants(eventID string, participants []models.EventParticipant) ([]models.EventParticipant, error) {
	if err := s.EnsurePeopleExist(participants); err != nil {
		return nil, err
	}
	if err := s.repo.SetParticipants(eventID, participants); err != nil {
		return nil, err
	}
	return s.repo.ListParticipants(eventID)
}

// RemoveParticipant drops one attendee. The last participant cannot be removed:
// a record nobody attended would be unreachable from every timeline.
func (s *EventService) RemoveParticipant(eventID, personID string) ([]models.EventParticipant, error) {
	current, err := s.ListParticipants(eventID)
	if err != nil {
		return nil, err
	}
	remaining := make([]models.EventParticipant, 0, len(current))
	found := false
	for _, p := range current {
		if p.PersonID == personID {
			found = true
			continue
		}
		remaining = append(remaining, p)
	}
	if !found {
		return nil, models.NewError(models.ErrNotFound, "person %s is not a participant of event %s", personID, eventID)
	}
	return s.SetParticipants(eventID, remaining)
}

// EnsurePeopleExist validates a record's attendance before the AI pipeline runs,
// so a bad participant id fails fast instead of after an LLM round trip.
func (s *EventService) EnsurePeopleExist(participants []models.EventParticipant) error {
	for _, p := range participants {
		if _, err := s.persons.GetByID(p.PersonID); err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return models.NewError(models.ErrPersonNotFound, "person %s not found", p.PersonID)
			}
			return err
		}
	}
	return nil
}
