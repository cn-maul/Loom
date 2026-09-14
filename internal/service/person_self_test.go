package service

import (
	"errors"
	"path/filepath"
	"testing"

	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
)

func newPersonSelfFixture(t *testing.T) *PersonService {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "person_self_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	// The vector index is created on demand; init it so a delete's index
	// cleanup has its table.
	vec := repository.NewVecRepo(database)
	if err := vec.Init(8); err != nil {
		t.Fatal(err)
	}
	return NewPersonService(
		repository.NewPersonRepo(database),
		vec,
		repository.NewOrganizationRepo(database),
	)
}

// The reserved self person is seeded exactly once, survives re-seeding with the
// same identity, and shows up in the ordinary person list.
func TestEnsureSelfSeedsOnceAndLists(t *testing.T) {
	svc := newPersonSelfFixture(t)
	if err := svc.EnsureSelf(); err != nil {
		t.Fatalf("first EnsureSelf: %v", err)
	}
	first, err := svc.repo.GetSelf()
	if err != nil {
		t.Fatalf("GetSelf after seed: %v", err)
	}
	if first.Name != SelfName || first.IsSelf != 1 {
		t.Fatalf("seeded self = %+v, want name %q is_self=1", first, SelfName)
	}

	if err := svc.EnsureSelf(); err != nil {
		t.Fatalf("second EnsureSelf: %v", err)
	}
	second, err := svc.repo.GetSelf()
	if err != nil {
		t.Fatalf("GetSelf after reseed: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("re-seeding replaced the self person: %s vs %s", first.ID, second.ID)
	}

	persons, _, err := svc.List(models.PersonFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range persons {
		if p.ID == first.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("the self person must appear in the ordinary person list")
	}
}

// The self person anchors every future record's perspective; deleting it would
// break that, so it is refused even though other persons delete freely.
func TestSelfPersonIsNotDeletable(t *testing.T) {
	svc := newPersonSelfFixture(t)
	if err := svc.EnsureSelf(); err != nil {
		t.Fatal(err)
	}
	self, err := svc.repo.GetSelf()
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(self.ID); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("deleting self = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.repo.GetByID(self.ID); err != nil {
		t.Fatalf("the self person must survive a refused delete: %v", err)
	}

	other := &models.Person{ID: "p1", Name: "张三", Importance: 3}
	if err := svc.Create(other); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(other.ID); err != nil {
		t.Fatalf("a normal person must stay deletable: %v", err)
	}
}
