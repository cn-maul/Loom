package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"relationship/internal/config"
	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
)

// structureContext is the piece that closes the "I marked them as my manager"
// loop: a relationship edge and a current posting must render into plain text
// the advice prompt can actually read. This test pins that rendering down so a
// future refactor cannot silently drop a direction marker or a role.
func TestStructureContextRendersRelationsAndPositions(t *testing.T) {
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "structure_context_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	personRepo := repository.NewPersonRepo(database)
	eventRepo := repository.NewEventRepo(database)
	relRepo := repository.NewRelationshipRepo(database)
	posRepo := repository.NewPositionRepo(database)

	// The person whose advice context we are building.
	me := &models.Person{ID: uuid.New().String(), Name: "我"}
	if err := personRepo.Create(me); err != nil {
		t.Fatal(err)
	}
	boss := &models.Person{ID: uuid.New().String(), Name: "张总"}
	if err := personRepo.Create(boss); err != nil {
		t.Fatal(err)
	}
	org := &models.Organization{ID: uuid.New().String(), Name: "产品部"}
	if err := repository.NewOrganizationRepo(database).Create(org); err != nil {
		t.Fatal(err)
	}

	// A directed edge "我 → 张总 (上级)", backed by an event so it is traceable.
	event := &models.Event{ID: uuid.New().String(), PersonID: me.ID, RawText: "张总是我的直属上级", EventDate: "2026-09-01"}
	if err := eventRepo.Create(event); err != nil {
		t.Fatal(err)
	}
	rel := &models.Relationship{
		ID:            uuid.New().String(),
		FromPersonID:  me.ID,
		ToPersonID:    boss.ID,
		RelationType:  "上级",
		Direction:     models.DirectionDirected,
		SourceEventID: event.ID,
	}
	if err := relRepo.Create(rel); err != nil {
		t.Fatal(err)
	}

	// A current posting: 我 is an engineer at 产品部.
	pos := &models.OrgPosition{
		ID:       uuid.New().String(),
		PersonID: me.ID,
		OrgID:    org.ID,
		Role:     "后端工程师",
	}
	if err := posRepo.Create(pos); err != nil {
		t.Fatal(err)
	}

	svc := &AIService{
		cfg: &config.LLMConfig{},
		// structureContext only reads relRepo and posRepo; the other fields can
		// stay nil.
		relRepo: relRepo,
		posRepo: posRepo,
	}

	rendered, cited := svc.structureContext(me)
	if !strings.Contains(rendered, "我 → 张总") {
		t.Fatalf("directed relation missing the direction arrow: %q", rendered)
	}
	if !strings.Contains(rendered, "上级") {
		t.Fatalf("relation type missing: %q", rendered)
	}
	if !strings.Contains(rendered, "产品部") || !strings.Contains(rendered, "后端工程师") {
		t.Fatalf("current posting not rendered: %q", rendered)
	}
	if !strings.Contains(rendered, "（未确认）") {
		t.Fatalf("unconfirmed edge must be marked: %q", rendered)
	}
	if len(cited) != 1 || cited[0] != event.ID {
		t.Fatalf("a relation citing its source event must fold it into evidence: %v", cited)
	}
}

// When there is nothing to show, the context must render empty rather than a
// half-written "关系：" header, so the prompt's "无" fallback stays honest.
func TestStructureContextEmpty(t *testing.T) {
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "structure_context_empty_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	personRepo := repository.NewPersonRepo(database)
	me := &models.Person{ID: uuid.New().String(), Name: "我"}
	if err := personRepo.Create(me); err != nil {
		t.Fatal(err)
	}

	svc := &AIService{
		cfg:     &config.LLMConfig{},
		relRepo: repository.NewRelationshipRepo(database),
		posRepo: repository.NewPositionRepo(database),
	}

	rendered, cited := svc.structureContext(me)
	if rendered != "" {
		t.Fatalf("empty context should be empty, got %q", rendered)
	}
	if len(cited) != 0 {
		t.Fatalf("empty context should cite nothing, got %v", cited)
	}
}