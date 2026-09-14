package service

import (
	"errors"
	"strings"

	"relationship/internal/models"
	"relationship/internal/repository"

	"github.com/google/uuid"
)

// SelfName is the reserved person representing the user. The user is not a
// contact, but records are written from their perspective and may name them as
// the subject or a participant, so exactly one is seeded at startup.
const SelfName = "我"

type PersonService struct {
	repo    *repository.PersonRepo
	vec     *repository.VecRepo
	orgRepo *repository.OrganizationRepo
}

func NewPersonService(repo *repository.PersonRepo, vec *repository.VecRepo, orgRepo *repository.OrganizationRepo) *PersonService {
	return &PersonService{repo: repo, vec: vec, orgRepo: orgRepo}
}

func (s *PersonService) Create(person *models.Person) error {
	if strings.TrimSpace(person.Name) == "" {
		return models.NewError(models.ErrInvalidInput, "name is required")
	}
	if person.Importance == 0 {
		person.Importance = 3
	}
	if err := validateImportance(person.Importance); err != nil {
		return err
	}
	if err := validateSelfFlag(person.IsSelf); err != nil {
		return err
	}
	if err := s.ensureOrg(person.OrgID); err != nil {
		return err
	}
	return s.repo.Create(person)
}

func (s *PersonService) GetByID(id string) (*models.Person, error) {
	return s.repo.GetByID(id)
}

// List returns the person list page: keyword, affiliation and relation
// filters, one of the closed ordering choices, and the page window. An unknown
// sort is a caller mistake and rejected, not silently mapped to a default.
func (s *PersonService) List(filter models.PersonFilter) ([]*models.PersonWithActivity, int, error) {
	if filter.Limit < 0 || filter.Offset < 0 {
		return nil, 0, models.NewError(models.ErrInvalidInput, "limit and offset must not be negative")
	}
	switch filter.Sort {
	case "", models.PersonSortRecent, models.PersonSortName, models.PersonSortImportance, models.PersonSortCreated:
	default:
		return nil, 0, models.NewError(models.ErrInvalidInput, "unknown sort %q", filter.Sort)
	}
	return s.repo.ListFiltered(filter)
}

func (s *PersonService) Update(person *models.Person) error {
	if person.ID == "" {
		return models.NewError(models.ErrInvalidInput, "id is required")
	}
	if strings.TrimSpace(person.Name) == "" {
		return models.NewError(models.ErrInvalidInput, "name is required")
	}
	if person.Importance == 0 {
		person.Importance = 3
	}
	if err := validateImportance(person.Importance); err != nil {
		return err
	}
	if err := validateSelfFlag(person.IsSelf); err != nil {
		return err
	}
	if err := s.ensureOrg(person.OrgID); err != nil {
		return err
	}
	return s.repo.Update(person)
}

// EnsureSelf seeds the reserved self person when the database has none, so “我”
// exists as an anchor and participant candidate from the first launch. Idempotent.
func (s *PersonService) EnsureSelf() error {
	_, err := s.repo.GetSelf()
	if err == nil {
		return nil
	}
	if !errors.Is(err, models.ErrNotFound) {
		return err
	}
	return s.repo.Create(&models.Person{
		ID:         uuid.New().String(),
		Name:       SelfName,
		IsSelf:     1,
		Importance: 3,
	})
}

// Delete drops the person. Records they attended survive as long as another
// participant remains, because events.person_id is ON DELETE SET NULL and the
// attendance rows cascade; events and traits they owned outright still cascade.
// The reserved self person is not deletable: every future record is written
// from their perspective.
func (s *PersonService) Delete(id string) error {
	person, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if person.IsSelf == 1 {
		return models.NewError(models.ErrInvalidInput, "「我」是系统保留人物，不能删除")
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	return s.vec.DeleteByPerson(id)
}

// ensureOrg rejects dangling affiliations instead of relying on the foreign key.
func (s *PersonService) ensureOrg(orgID string) error {
	if orgID == "" {
		return nil
	}
	if _, err := s.orgRepo.GetByID(orgID); err != nil {
		return models.NewError(models.ErrOrgNotFound, "organization %s not found", orgID)
	}
	return nil
}

func validateImportance(importance int) error {
	if importance < 1 || importance > 5 {
		return models.NewError(models.ErrInvalidInput, "importance must be between 1 and 5")
	}
	return nil
}

// validateSelfFlag keeps is_self a flag, not a count: exactly one person can be
// the user, and the repository clears any previous holder atomically.
func validateSelfFlag(isSelf int) error {
	if isSelf != 0 && isSelf != 1 {
		return models.NewError(models.ErrInvalidInput, "is_self must be 0 or 1")
	}
	return nil
}
