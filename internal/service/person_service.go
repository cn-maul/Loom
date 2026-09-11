package service

import (
	"strings"

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

// Delete drops the person. Records they attended survive as long as another
// participant remains, because events.person_id is ON DELETE SET NULL and the
// attendance rows cascade; events and traits they owned outright still cascade.
func (s *PersonService) Delete(id string) error {
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
