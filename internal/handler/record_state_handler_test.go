package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"relationship/internal/ai"
	"relationship/internal/config"
	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
	"relationship/internal/service"
)

type recordFixture struct {
	e      *echo.Echo
	events *repository.EventRepo
	traits *repository.TraitRepo
	person *models.Person
}

// newRecordAPI wires the record routes against a throwaway database. Records are
// seeded through the repository so the fixture never needs a language model; the
// AI service points at a closed port and is only reached on paths the tests do
// not exercise.
func newRecordAPI(t *testing.T) *recordFixture {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "record_handler_test.db"))
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
	persons := service.NewPersonService(repository.NewPersonRepo(database), vec, repository.NewOrganizationRepo(database))
	person := &models.Person{ID: uuid.New().String(), Name: "张总"}
	if err := persons.Create(person); err != nil {
		t.Fatal(err)
	}

	eventRepo := repository.NewEventRepo(database)
	traitRepo := repository.NewTraitRepo(database)
	eventService := service.NewEventService(eventRepo, vec, persons, traitRepo)

	cfg := &config.LLMConfig{Endpoint: "http://127.0.0.1:1", ExtractModel: "test"}
	aiService := service.NewAIService(cfg, vec, traitRepo, eventRepo, repository.NewPersonRepo(database), ai.NewClient(cfg))
	// Synchronous ingest for tests: no background queue to wait on.
	ingest := service.NewIngestService(aiService, &config.LLMConfig{AsyncExtract: false})
	t.Cleanup(ingest.Stop)

	eventHandler := NewEventHandler(eventService, aiService, ingest)
	traitHandler := NewTraitHandler(traitRepo)
	e := echo.New()
	api := e.Group("/api")
	api.GET("/events", eventHandler.List)
	api.GET("/events/:id", eventHandler.GetByID)
	api.PUT("/events/:id", eventHandler.Update)
	api.DELETE("/events/:id", eventHandler.Delete)
	api.POST("/events/:id/extract", eventHandler.RetryExtract)
	api.GET("/persons/:id/traits", traitHandler.ListByPerson)

	return &recordFixture{e: e, events: eventRepo, traits: traitRepo, person: person}
}

func (f *recordFixture) seed(t *testing.T, id, rawText string) {
	t.Helper()
	f.seedOn(t, id, rawText, "2026-09-01")
}

func (f *recordFixture) seedOn(t *testing.T, id, rawText, date string) {
	t.Helper()
	event := &models.Event{ID: id, PersonID: f.person.ID, RawText: rawText, EventDate: date}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := f.events.Create(event); err != nil {
		t.Fatal(err)
	}
}

func (f *recordFixture) seedTrait(t *testing.T, eventID string) {
	t.Helper()
	if err := f.traits.Upsert(&models.Trait{
		ID: uuid.New().String(), PersonID: f.person.ID, TraitKey: "沟通风格", TraitValue: "直接",
		SourceEventIDs: models.EncodeIDList([]string{eventID}),
	}); err != nil {
		t.Fatal(err)
	}
}

func decodeEvent(t *testing.T, resp models.APIResponse) models.Event {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatal(err)
	}
	var event models.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	return event
}

// The record API reports what the pipeline did, lets the list pick out the
// records that need attention, and refuses to overwrite a human correction.
func TestEventAPIExposesExtractionState(t *testing.T) {
	f := newRecordAPI(t)
	f.seed(t, "e1", "开会聊了合同")

	status, resp := doJSON(t, f.e, http.MethodGet, "/api/events/e1", "")
	if status != http.StatusOK {
		t.Fatalf("read returned %d: %+v", status, resp.Error)
	}
	if event := decodeEvent(t, resp); event.ExtractionStatus != models.ExtractionPending || event.ManuallyEdited != 0 {
		t.Fatalf("a fresh record should be pending and untouched: %+v", event)
	}

	status, resp = doJSON(t, f.e, http.MethodGet, "/api/events?status=pending", "")
	if status != http.StatusOK {
		t.Fatalf("filtering by extraction state returned %d: %+v", status, resp.Error)
	}
	if items, _ := resp.Data.([]any); len(items) != 1 {
		t.Fatalf("want 1 pending record, got %d", len(items))
	}
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/events?status=done", "")
	if status != http.StatusBadRequest || resp.Error == nil || resp.Error.Code != "INVALID_INPUT" {
		t.Fatalf("unknown extraction state = %d %+v, want 400 INVALID_INPUT", status, resp.Error)
	}

	status, resp = doJSON(t, f.e, http.MethodPut, "/api/events/e1",
		`{"person_id":"`+f.person.ID+`","raw_text":"开会聊了合同","event_date":"2026-09-01","summary":"我自己写的摘要"}`)
	if status != http.StatusOK {
		t.Fatalf("edit returned %d: %+v", status, resp.Error)
	}
	edited := decodeEvent(t, resp)
	if edited.ManuallyEdited != 1 || edited.EditedAt == nil {
		t.Fatalf("a hand edit should be flagged: %+v", edited)
	}

	// The retry is refused before the extractor is even contacted.
	status, resp = doJSON(t, f.e, http.MethodPost, "/api/events/e1/extract", "")
	if status != http.StatusConflict || resp.Error == nil || resp.Error.Code != "CONFLICT" {
		t.Fatalf("retry over a hand-edited record = %d %+v, want 409 CONFLICT", status, resp.Error)
	}

	status, resp = doJSON(t, f.e, http.MethodPost, "/api/events/"+uuid.New().String()+"/extract", "")
	if status != http.StatusNotFound {
		t.Fatalf("retry of a missing record = %d %+v, want 404", status, resp.Error)
	}
}

// The record list is paged, so the whole match count has to travel beside the
// window: the caller cannot tell a last page from an empty one otherwise.
func TestEventListPagesAndReportsTotal(t *testing.T) {
	f := newRecordAPI(t)
	f.seedOn(t, "e1", "开会聊了合同", "2026-09-01")
	f.seedOn(t, "e2", "一起吃了个饭", "2026-09-05")
	f.seedOn(t, "e3", "电话里敲定价格", "2026-09-09")

	total, ids := eventListPage(t, f.e, "/api/events?limit=2")
	if total != 3 {
		t.Fatalf("X-Total-Count = %d, want 3", total)
	}
	// Newest first, so the first page holds the two most recent records.
	if len(ids) != 2 || ids[0] != "e3" || ids[1] != "e2" {
		t.Fatalf("first page = %v, want [e3 e2]", ids)
	}

	total, ids = eventListPage(t, f.e, "/api/events?limit=2&offset=2")
	if total != 3 || len(ids) != 1 || ids[0] != "e1" {
		t.Fatalf("second page = %v (total %d), want [e1] over 3", ids, total)
	}

	total, ids = eventListPage(t, f.e, "/api/events?q="+url.QueryEscape("价格"))
	if total != 1 || len(ids) != 1 || ids[0] != "e3" {
		t.Fatalf("keyword search = %v (total %d), want [e3] over 1", ids, total)
	}

	total, ids = eventListPage(t, f.e, "/api/events?from=2026-09-05")
	if total != 2 || len(ids) != 2 {
		t.Fatalf("date window = %v (total %d), want 2 records from 09-05 on", ids, total)
	}
}

// eventListPage runs one list request and returns the total header together with
// the ids on the page, in the order they came back.
func eventListPage(t *testing.T, e *echo.Echo, target string) (int, []string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s returned %d: %s", target, rec.Code, rec.Body.String())
	}
	var resp models.APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	total, err := strconv.Atoi(rec.Header().Get("X-Total-Count"))
	if err != nil {
		t.Fatalf("X-Total-Count = %q, want a number", rec.Header().Get("X-Total-Count"))
	}
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatal(err)
	}
	var events []models.Event
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return total, ids
}

// Removing the evidence behind a profile note marks the note, it does not hide it.
func TestEventAPIDeleteFlagsDerivedProfileNote(t *testing.T) {
	f := newRecordAPI(t)
	f.seed(t, "e1", "开会聊了合同")
	f.seedTrait(t, "e1")

	status, resp := doJSON(t, f.e, http.MethodDelete, "/api/events/e1", "")
	if status != http.StatusOK {
		t.Fatalf("delete returned %d: %+v", status, resp.Error)
	}

	status, resp = doJSON(t, f.e, http.MethodGet, "/api/persons/"+f.person.ID+"/traits", "")
	if status != http.StatusOK {
		t.Fatalf("reading traits returned %d: %+v", status, resp.Error)
	}
	traits, _ := resp.Data.([]any)
	if len(traits) != 1 {
		t.Fatalf("the note should stay visible, got %d", len(traits))
	}
	note, _ := traits[0].(map[string]any)
	if note["source_stale"] != float64(1) {
		t.Fatalf("the note was not flagged as stale: %+v", note)
	}
	if reason, _ := note["source_stale_reason"].(string); reason == "" {
		t.Fatalf("a stale note must say why: %+v", note)
	}
}
