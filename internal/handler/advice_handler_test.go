package handler

import (
	"encoding/json"
	"net/http"
	"path/filepath"
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

type adviceFixture struct {
	e         *echo.Echo
	advice    *repository.AdviceRepo
	followUps *repository.FollowUpRepo
	events    *repository.EventRepo
	person    *models.Person
}

// newAdviceAPI wires the advice routes. Sessions are seeded through the
// repository so the fixture never needs a language model: generation is the only
// path that reaches a provider, and the tests below enter the service after that
// point.
func newAdviceAPI(t *testing.T) *adviceFixture {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "advice_handler_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	vec := repository.NewVecRepo(database)
	personRepo := repository.NewPersonRepo(database)
	eventRepo := repository.NewEventRepo(database)
	traitRepo := repository.NewTraitRepo(database)
	adviceRepo := repository.NewAdviceRepo(database)
	followUpRepo := repository.NewFollowUpRepo(database)

	persons := service.NewPersonService(personRepo, vec, repository.NewOrganizationRepo(database))
	person := &models.Person{ID: uuid.New().String(), Name: "张总"}
	if err := persons.Create(person); err != nil {
		t.Fatal(err)
	}

	cfg := &config.LLMConfig{Endpoint: "http://127.0.0.1:1", ExtractModel: "test", AdviceModel: "test"}
	aiService := service.NewAIService(cfg, vec, traitRepo, eventRepo, personRepo, ai.NewClient(cfg))
	adviceService := service.NewAdviceService(adviceRepo, eventRepo, followUpRepo, persons, aiService)

	handler := NewAdviceHandler(adviceService)
	e := echo.New()
	api := e.Group("/api")
	api.POST("/advice", handler.Generate)
	api.GET("/advice", handler.List)
	api.GET("/advice/:id", handler.Get)
	api.DELETE("/advice/:id", handler.Delete)
	api.POST("/advice/:id/adopt", handler.Adopt)

	return &adviceFixture{e: e, advice: adviceRepo, followUps: followUpRepo, events: eventRepo, person: person}
}

// seedSession stores an answer directly, which is what a user coming back to
// yesterday's advice is reading.
func (f *adviceFixture) seedSession(t *testing.T, id string) {
	t.Helper()
	session := &models.AdviceSession{
		ID:       id,
		PersonID: f.person.ID,
		Question: "怎么跟他提延期",
		Goal:     "别把关系搞僵",
		AdviceResponse: models.AdviceResponse{
			Situation: models.AdvicePoint{
				Text:             "他最近在回避这个话题",
				EvidenceEventIDs: []string{"e1"},
			},
			Risks: []models.AdvicePoint{{Text: "把话说硬了会伤关系", EvidenceEventIDs: []string{"e1"}}},
			Strategies: []models.Strategy{
				{Name: "先共情再提事", Script: "最近是不是挺累的？", Pros: "降低对抗", Cons: "见效慢"},
			},
			FollowUp:         models.AdvicePoint{Text: "三天后再确认一次"},
			EvidenceEventIDs: []string{"e1"},
			RetrievalStatus:  models.RetrievalVectorDisabled,
		},
		UsedEventIDs:    []string{"e1"},
		EvidenceVersion: []models.EvidenceVersion{{EventID: "e1", ContentRev: "rev1"}},
		Model:           "test",
		FollowUpIDs:     []string{},
	}
	if err := f.advice.Create(session); err != nil {
		t.Fatal(err)
	}
}

func decodeAdviceSession(t *testing.T, resp models.APIResponse) models.AdviceSession {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatal(err)
	}
	var session models.AdviceSession
	if err := json.Unmarshal(raw, &session); err != nil {
		t.Fatal(err)
	}
	return session
}

// A stored answer is readable again, and the per-conclusion evidence survives the
// round trip through HTTP, not only through the repository.
func TestAdviceAPIReadsStoredSession(t *testing.T) {
	f := newAdviceAPI(t)
	f.seedSession(t, "a1")

	status, resp := doJSON(t, f.e, http.MethodGet, "/api/advice/a1", "")
	if status != http.StatusOK {
		t.Fatalf("read returned %d: %+v", status, resp.Error)
	}
	session := decodeAdviceSession(t, resp)
	if session.Question != "怎么跟他提延期" || session.Goal != "别把关系搞僵" {
		t.Fatalf("session = %+v", session)
	}
	if session.Situation.Text != "他最近在回避这个话题" {
		t.Fatalf("situation = %q", session.Situation.Text)
	}
	if len(session.Situation.EvidenceEventIDs) != 1 || len(session.Strategies) != 1 {
		t.Fatalf("the answer lost its structure: %+v", session)
	}
	if session.RetrievalStatus != models.RetrievalVectorDisabled {
		t.Fatalf("retrieval status = %q", session.RetrievalStatus)
	}

	status, resp = doJSON(t, f.e, http.MethodGet, "/api/advice?person_id="+f.person.ID, "")
	if status != http.StatusOK {
		t.Fatalf("history returned %d: %+v", status, resp.Error)
	}
	if items, _ := resp.Data.([]any); len(items) != 1 {
		t.Fatalf("history = %d rows, want 1", len(items))
	}

	status, resp = doJSON(t, f.e, http.MethodGet, "/api/advice/"+uuid.New().String(), "")
	if status != http.StatusNotFound || resp.Error == nil || resp.Error.Code != "NOT_FOUND" {
		t.Fatalf("a missing session = %d %+v, want 404 NOT_FOUND", status, resp.Error)
	}
}

// Adopting through the API has to leave a real follow-up behind, with the
// strategy's words on it.
func TestAdviceAPIAdoptCreatesFollowUp(t *testing.T) {
	f := newAdviceAPI(t)
	f.seedSession(t, "a1")

	status, resp := doJSON(t, f.e, http.MethodPost, "/api/advice/a1/adopt",
		`{"strategy_index":0,"due_text":"这周内"}`)
	if status != http.StatusCreated {
		t.Fatalf("adopt returned %d: %+v", status, resp.Error)
	}
	session := decodeAdviceSession(t, resp)
	if session.AdoptedStrategyName != "先共情再提事" || len(session.FollowUpIDs) != 1 {
		t.Fatalf("adopted session = %+v", session)
	}

	followUp, err := f.followUps.Get(session.FollowUpIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if followUp.Title != "先共情再提事" || followUp.Description != "最近是不是挺累的？" {
		t.Fatalf("the item did not carry the strategy over: %+v", followUp)
	}
	if followUp.SourceAdviceID != "a1" || followUp.DueText != "这周内" {
		t.Fatalf("follow-up = %+v", followUp)
	}

	status, resp = doJSON(t, f.e, http.MethodPost, "/api/advice/a1/adopt", `{"strategy_index":5}`)
	if status != http.StatusBadRequest || resp.Error == nil || resp.Error.Code != "INVALID_INPUT" {
		t.Fatalf("an out-of-range strategy = %d %+v, want 400 INVALID_INPUT", status, resp.Error)
	}
	status, resp = doJSON(t, f.e, http.MethodPost, "/api/advice/"+uuid.New().String()+"/adopt", `{"strategy_index":0}`)
	if status != http.StatusNotFound {
		t.Fatalf("adopting from a missing session = %d %+v, want 404", status, resp.Error)
	}
}

// Deleting the reasoning behind an action leaves the action standing with a note.
func TestAdviceAPIDeleteKeepsAdoptedFollowUp(t *testing.T) {
	f := newAdviceAPI(t)
	f.seedSession(t, "a1")

	status, resp := doJSON(t, f.e, http.MethodPost, "/api/advice/a1/adopt", `{"strategy_index":0}`)
	if status != http.StatusCreated {
		t.Fatalf("adopt returned %d: %+v", status, resp.Error)
	}
	followUpID := decodeAdviceSession(t, resp).FollowUpIDs[0]

	status, resp = doJSON(t, f.e, http.MethodDelete, "/api/advice/a1", "")
	if status != http.StatusOK {
		t.Fatalf("delete returned %d: %+v", status, resp.Error)
	}
	status, _ = doJSON(t, f.e, http.MethodGet, "/api/advice/a1", "")
	if status != http.StatusNotFound {
		t.Fatalf("the session should be gone, got %d", status)
	}

	followUp, err := f.followUps.Get(followUpID)
	if err != nil {
		t.Fatal("the follow-up must survive its advice being deleted")
	}
	if followUp.SourceAdviceStale != 1 || followUp.SourceAdviceStaleReason == "" {
		t.Fatalf("the item does not remember its advice is gone: %+v", followUp)
	}

	status, resp = doJSON(t, f.e, http.MethodDelete, "/api/advice/"+uuid.New().String(), "")
	if status != http.StatusNotFound {
		t.Fatalf("deleting a missing session = %d %+v, want 404", status, resp.Error)
	}
}

// Validation has to come out as 400, and an unknown person as 404 rather than a
// 500 from a provider round trip.
func TestAdviceAPIGenerateValidatesInput(t *testing.T) {
	f := newAdviceAPI(t)

	status, resp := doJSON(t, f.e, http.MethodPost, "/api/advice", `{"person_id":"`+f.person.ID+`"}`)
	if status != http.StatusBadRequest || resp.Error == nil || resp.Error.Code != "INVALID_INPUT" {
		t.Fatalf("missing question = %d %+v, want 400 INVALID_INPUT", status, resp.Error)
	}

	status, resp = doJSON(t, f.e, http.MethodPost, "/api/advice", `{"person_id":"ghost","question":"?"}`)
	if status != http.StatusNotFound || resp.Error == nil || resp.Error.Code != "PERSON_NOT_FOUND" {
		t.Fatalf("unknown person = %d %+v, want 404 PERSON_NOT_FOUND", status, resp.Error)
	}
}
