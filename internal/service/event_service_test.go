package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"relationship/internal/ai"
	"relationship/internal/config"
	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
)

type eventFixture struct {
	svc    *EventService
	ai     *AIService
	events *repository.EventRepo
	traits *repository.TraitRepo
	person *models.Person
}

func newEventFixture(t *testing.T) *eventFixture {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "event_service_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	vec := repository.NewVecRepo(database)
	if err := vec.Init(4); err != nil {
		t.Fatal(err)
	}
	persons := NewPersonService(repository.NewPersonRepo(database), vec, repository.NewOrganizationRepo(database))
	person := &models.Person{ID: uuid.New().String(), Name: "张总"}
	if err := persons.Create(person); err != nil {
		t.Fatal(err)
	}

	eventRepo := repository.NewEventRepo(database)
	traitRepo := repository.NewTraitRepo(database)
	// The endpoint is never reached by the assertions that use this service: the
	// manual-edit guard fires before any request is made.
	cfg := &config.LLMConfig{Endpoint: "http://127.0.0.1:1", ExtractModel: "test"}
	client := ai.NewClient(cfg)

	return &eventFixture{
		svc:    NewEventService(eventRepo, vec, persons, traitRepo),
		ai:     NewAIService(cfg, vec, traitRepo, eventRepo, repository.NewPersonRepo(database), repository.NewRelationshipRepo(database), repository.NewPositionRepo(database), client),
		events: eventRepo,
		traits: traitRepo,
		person: person,
	}
}

func (f *eventFixture) seed(t *testing.T, id, rawText string) *models.Event {
	t.Helper()
	event := &models.Event{ID: id, PersonID: f.person.ID, RawText: rawText, EventDate: "2026-09-01"}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := f.events.Create(event); err != nil {
		t.Fatal(err)
	}
	return event
}

func (f *eventFixture) seedTrait(t *testing.T, eventID string) {
	t.Helper()
	if err := f.traits.Upsert(&models.Trait{
		ID: uuid.New().String(), PersonID: f.person.ID, TraitKey: "沟通风格", TraitValue: "直接",
		SourceEventIDs: models.EncodeIDList([]string{eventID}),
	}); err != nil {
		t.Fatal(err)
	}
}

// Editing what the person actually said invalidates the profile notes drawn from
// it; editing only the derived summary does not.
func TestEventUpdateMarksDerivedProfileStale(t *testing.T) {
	f := newEventFixture(t)
	event := f.seed(t, "e1", "原先的原文")
	f.seedTrait(t, "e1")

	edited := *event
	edited.Summary = "换个摘要"
	if err := f.svc.Update(&edited); err != nil {
		t.Fatal(err)
	}
	trait, err := f.traits.GetByKey(f.person.ID, "沟通风格")
	if err != nil {
		t.Fatal(err)
	}
	if trait.SourceStale != 0 {
		t.Fatal("rewriting a derived summary leaves the evidence intact and must not flag the note")
	}

	edited.RawText = "改过的原文"
	if err := f.svc.Update(&edited); err != nil {
		t.Fatal(err)
	}
	trait, err = f.traits.GetByKey(f.person.ID, "沟通风格")
	if err != nil {
		t.Fatal(err)
	}
	if trait.SourceStale != 1 || trait.SourceStaleReason == "" {
		t.Fatalf("a rewritten source should flag the note, got %+v", trait)
	}
}

// A retry rewrites the extracted fields of a record that was just re-read by
// the model. Flagging the profile here would undo the refresh the retry is
// supposed to trigger, so only the user's own edits mark a note stale.
func TestUpdateExtractionKeepsProfileEvidence(t *testing.T) {
	f := newEventFixture(t)
	event := f.seed(t, "e1", "原先的原文")
	f.seedTrait(t, "e1")

	edited := *event
	edited.Summary = "模型给的新摘要"
	if err := f.events.UpdateExtraction(&edited); err != nil {
		t.Fatal(err)
	}
	trait, err := f.traits.GetByKey(f.person.ID, "沟通风格")
	if err != nil {
		t.Fatal(err)
	}
	if trait.SourceStale != 0 {
		t.Fatalf("re-running extraction must not flag the profile, got %+v", trait)
	}
}

// Deleting the record is the strongest form of "the source changed". The note
// stays visible and marked rather than disappearing.
func TestEventDeleteMarksDerivedProfileStale(t *testing.T) {
	f := newEventFixture(t)
	f.seed(t, "e1", "开会")
	f.seedTrait(t, "e1")

	if err := f.svc.Delete("e1"); err != nil {
		t.Fatal(err)
	}
	traits, err := f.traits.ListByPerson(f.person.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(traits) != 1 {
		t.Fatalf("the note should be kept and flagged, got %d rows", len(traits))
	}
	if traits[0].SourceStale != 1 || traits[0].SourceStaleReason == "" {
		t.Fatalf("deleting the source should flag the note, got %+v", traits[0])
	}
}

// A retry must not silently replace a correction a human made. The guard fires
// before the extractor is contacted, which is why a dead endpoint is fine here.
func TestRetryExtractionProtectsManualEdits(t *testing.T) {
	f := newEventFixture(t)
	f.seed(t, "e1", "开会")
	f.recordManualEdit(t, "e1")

	if _, _, err := f.ai.RetryExtraction(context.Background(), "e1", false); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("retry over a hand-edited record = %v, want ErrConflict", err)
	}
	if _, _, err := f.ai.RetryExtraction(context.Background(), "ghost", true); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("retry of a missing record = %v, want ErrNotFound", err)
	}
}

func (f *eventFixture) recordManualEdit(t *testing.T, eventID string) {
	t.Helper()
	event, err := f.events.GetByID(eventID)
	if err != nil {
		t.Fatal(err)
	}
	event.Summary = "我自己写的摘要"
	if err := f.events.Update(event); err != nil {
		t.Fatal(err)
	}
}
