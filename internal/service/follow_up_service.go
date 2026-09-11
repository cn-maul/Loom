package service

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"relationship/internal/models"
	"relationship/internal/repository"
)

type FollowUpService struct {
	repo    *repository.FollowUpRepo
	persons *PersonService
	events  *repository.EventRepo
}

func NewFollowUpService(repo *repository.FollowUpRepo, persons *PersonService, events *repository.EventRepo) *FollowUpService {
	return &FollowUpService{repo: repo, persons: persons, events: events}
}

// storedFollowUpStatuses is the closed set the schema and the API agree on.
var storedFollowUpStatuses = map[string]bool{
	models.FollowUpPending: true, models.FollowUpWaiting: true,
	models.FollowUpCompleted: true, models.FollowUpCancelled: true,
}

// actionStatuses are the transitions the action endpoints may apply. "pending"
// is deliberately absent: returning work to pending is an explicit edit, not a
// one-click action, so a completed or cancelled item is not casually reopened.
var actionStatuses = map[string]bool{
	models.FollowUpWaiting: true, models.FollowUpCompleted: true, models.FollowUpCancelled: true,
}

func (s *FollowUpService) Create(f *models.FollowUp) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if err := s.ensurePerson(f.PersonID); err != nil {
		return err
	}
	if err := s.ensureEvent("source_event_id", f.SourceEventID); err != nil {
		return err
	}
	// A brand-new item has no outcome; accepting one here would let a caller
	// write a completion note onto an open follow-up.
	f.CompletionNote, f.CompletedEventID = "", ""
	f.ID = uuid.New().String()
	return s.repo.Create(f)
}

func (s *FollowUpService) Get(id string) (*models.FollowUp, error) {
	return s.repo.Get(id)
}

// ListPostponements answers the deadline history for one item, 404ing on an
// unknown id rather than returning an empty list that looks like "never moved".
func (s *FollowUpService) ListPostponements(id string) ([]models.Postponement, error) {
	if _, err := s.repo.Get(id); err != nil {
		return nil, err
	}
	return s.repo.ListPostponements(id)
}

// List validates the filters before they reach SQL, so a malformed range or
// status is a 400 instead of a silently wrong result set.
func (s *FollowUpService) List(personID, status, from, to string) ([]*models.FollowUp, error) {
	if status != "" && !storedFollowUpStatuses[status] {
		return nil, models.NewError(models.ErrInvalidInput, "unknown status %q", status)
	}
	if err := validateDateRange(from, to); err != nil {
		return nil, err
	}
	return s.repo.List(personID, status, from, to)
}

func (s *FollowUpService) Update(f *models.FollowUp) error {
	if f.ID == "" {
		return models.NewError(models.ErrInvalidInput, "id is required")
	}
	if err := f.Validate(); err != nil {
		return err
	}
	if err := s.ensureEvent("source_event_id", f.SourceEventID); err != nil {
		return err
	}
	if err := s.ensureEvent("completed_event_id", f.CompletedEventID); err != nil {
		return err
	}
	return s.repo.Update(f)
}

// Complete closes an item, optionally recording the outcome and the record
// written afterwards. Both ids are checked first, so a completion cannot leave
// a dangling reference behind.
func (s *FollowUpService) Complete(id, completionNote, completedEventID string) error {
	if err := s.ensureEvent("completed_event_id", completedEventID); err != nil {
		return err
	}
	return s.repo.Complete(id, completionNote, completedEventID)
}

func (s *FollowUpService) Postpone(id, dueDate, reason string) error {
	if err := validateDate("due_date", dueDate); err != nil {
		return err
	}
	return s.repo.Postpone(id, dueDate, reason)
}

func (s *FollowUpService) Action(id, status string) error {
	if !actionStatuses[status] {
		return models.NewError(models.ErrInvalidInput, "unknown status %q", status)
	}
	return s.repo.Action(id, status)
}

func (s *FollowUpService) ensurePerson(personID string) error {
	if _, err := s.persons.GetByID(personID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return models.NewError(models.ErrPersonNotFound, "person %s not found", personID)
		}
		return err
	}
	return nil
}

// ensureEvent checks an optional cross-reference. An empty id means "no link",
// which is a valid state rather than an error.
func (s *FollowUpService) ensureEvent(field, eventID string) error {
	if eventID == "" {
		return nil
	}
	if _, err := s.events.GetByID(eventID); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return models.NewError(models.ErrNotFound, "%s: event %s not found", field, eventID)
		}
		return err
	}
	return nil
}

func validateDate(field, value string) error {
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return models.NewError(models.ErrInvalidInput, "%s must be YYYY-MM-DD", field)
	}
	return nil
}

func validateDateRange(from, to string) error {
	if from != "" {
		if err := validateDate("from", from); err != nil {
			return err
		}
	}
	if to != "" {
		if err := validateDate("to", to); err != nil {
			return err
		}
	}
	if from != "" && to != "" && from > to {
		return models.NewError(models.ErrInvalidInput, "from must not be after to")
	}
	return nil
}
