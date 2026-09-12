package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"relationship/internal/ai"
	"relationship/internal/config"
	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
)

// adviceAnswer is what the stubbed model replies with. It deliberately cites one
// real record and one id that was never given to it, so the evidence filter has
// something to reject. EVENT_ID is substituted with the seeded record's id.
const adviceAnswer = `{
  "situation": {"text": "他最近在回避这个话题", "evidence_event_ids": ["EVENT_ID", "invented-id"]},
  "other_perspective": {"text": "他可能也在等一个台阶", "evidence_event_ids": []},
  "risks": [{"text": "把话说硬了会伤关系", "evidence_event_ids": ["EVENT_ID"]}],
  "strategies": [
    {"name": "先共情再提事", "script": "最近是不是挺累的？", "pros": "降低对抗", "cons": "见效慢", "evidence_event_ids": ["EVENT_ID"]},
    {"name": "给一个具体选项", "script": "要不我们约周三？", "pros": "直接", "cons": "可能被拒", "evidence_event_ids": []}
  ],
  "follow_up": {"text": "三天后再确认一次", "evidence_event_ids": []}
}`

// adviceEventID is the record the stub answer cites.
const adviceEventID = "e1"

type adviceFixture struct {
	svc       *AdviceService
	advice    *repository.AdviceRepo
	events    *repository.EventRepo
	followUps *repository.FollowUpRepo
	vec       *repository.VecRepo
	person    *models.Person
}

// newAdviceFixture stands up a stubbed provider. Embedding responses are
// injectable so the three retrieval outcomes can be exercised without a real
// embedding service: a usable vector, an empty result, and a broken endpoint.
func newAdviceFixture(t *testing.T, initVec bool, embedBody string) *adviceFixture {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "advice_service_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	personRepo := repository.NewPersonRepo(database)
	eventRepo := repository.NewEventRepo(database)
	traitRepo := repository.NewTraitRepo(database)
	adviceRepo := repository.NewAdviceRepo(database)
	followUpRepo := repository.NewFollowUpRepo(database)
	vec := repository.NewVecRepo(database)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/embeddings" {
			_, _ = w.Write([]byte(embedBody))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test", "object": "chat.completion", "model": "test",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": strings.ReplaceAll(adviceAnswer, "EVENT_ID", adviceEventID)},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	t.Cleanup(server.Close)

	cfg := &config.LLMConfig{
		Endpoint: server.URL, Protocol: "openai", APIKey: "test-key",
		ExtractModel: "test", AdviceModel: "test-model",
		EmbedModel: "test", EmbedDim: 4, MaxTokens: 2000,
	}
	if initVec {
		if err := vec.Init(cfg.EmbedDim); err != nil {
			t.Fatal(err)
		}
	}
	persons := NewPersonService(personRepo, vec, repository.NewOrganizationRepo(database))
	person := &models.Person{ID: uuid.New().String(), Name: "张总"}
	if err := persons.Create(person); err != nil {
		t.Fatal(err)
	}

	aiService := NewAIService(cfg, vec, traitRepo, eventRepo, personRepo, repository.NewRelationshipRepo(database), repository.NewPositionRepo(database), ai.NewClient(cfg))
	return &adviceFixture{
		svc:       NewAdviceService(adviceRepo, eventRepo, followUpRepo, persons, aiService),
		advice:    adviceRepo,
		events:    eventRepo,
		followUps: followUpRepo,
		vec:       vec,
		person:    person,
	}
}

const embedOK = `{"data":[{"embedding":[1,0,0,0]}]}`

func (f *adviceFixture) seedEvent(t *testing.T, id, rawText string) *models.Event {
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

func (f *adviceFixture) ask(t *testing.T) *models.AdviceSession {
	t.Helper()
	session, err := f.svc.Generate(context.Background(), models.AdviceRequest{
		PersonID: f.person.ID, Question: "怎么跟他提延期", Goal: "别把关系搞僵",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return session
}

// The answer is stored with its structure, and an evidence id the model made up
// is dropped rather than shown as if it pointed at a record.
func TestAdviceGeneratePersistsSession(t *testing.T) {
	f := newAdviceFixture(t, false, embedOK)
	f.seedEvent(t, "e1", "他说下周给答复")

	session := f.ask(t)

	if session.ID == "" || session.Question != "怎么跟他提延期" || session.Goal != "别把关系搞僵" {
		t.Fatalf("session = %+v", session)
	}
	if session.Situation.Text != "他最近在回避这个话题" {
		t.Fatalf("situation = %q", session.Situation.Text)
	}
	if len(session.Situation.EvidenceEventIDs) != 1 || session.Situation.EvidenceEventIDs[0] != "e1" {
		t.Fatalf("invented evidence survived: %v", session.Situation.EvidenceEventIDs)
	}
	if len(session.EvidenceEventIDs) != 1 || session.EvidenceEventIDs[0] != "e1" {
		t.Fatalf("union evidence = %v", session.EvidenceEventIDs)
	}
	if len(session.Strategies) != 2 {
		t.Fatalf("strategies = %d", len(session.Strategies))
	}
	if len(session.Strategies[1].EvidenceEventIDs) != 0 {
		t.Fatal("a strategy the model gave no evidence for must not inherit any")
	}
	if session.Model != "test-model" {
		t.Fatalf("model = %q", session.Model)
	}
	if len(session.UsedEventIDs) != 1 || session.UsedEventIDs[0] != "e1" {
		t.Fatalf("used events = %v", session.UsedEventIDs)
	}
	if len(session.EvidenceVersion) != 1 || session.EvidenceVersion[0].ContentRev == "" {
		t.Fatalf("the evidence snapshot was not taken: %+v", session.EvidenceVersion)
	}

	// It has to be findable again by id, which is the whole point of storing it.
	stored, err := f.svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Question != session.Question || len(stored.Strategies) != 2 {
		t.Fatalf("stored session = %+v", stored)
	}
	list, err := f.svc.List(f.person.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != session.ID {
		t.Fatalf("history = %+v", list)
	}
}

// "No index", "search broke" and "nothing matched" are three different things to
// tell the user, so they must not collapse into one boolean.
func TestAdviceGenerateDistinguishesRetrievalOutcomes(t *testing.T) {
	disabled := newAdviceFixture(t, false, embedOK)
	disabled.seedEvent(t, "e1", "开会")
	if got := disabled.ask(t).RetrievalStatus; got != models.RetrievalVectorDisabled {
		t.Fatalf("without a vector index the status = %q, want %q", got, models.RetrievalVectorDisabled)
	}

	empty := newAdviceFixture(t, true, embedOK)
	empty.seedEvent(t, "e1", "开会")
	if got := empty.ask(t).RetrievalStatus; got != models.RetrievalNoEvidence {
		t.Fatalf("an index with no matching rows = %q, want %q", got, models.RetrievalNoEvidence)
	}

	// The same index, now holding the record under the question's embedding.
	if err := empty.vec.Insert("event:e1", []float32{1, 0, 0, 0}, empty.person.ID, "event_summary", "e1"); err != nil {
		t.Fatal(err)
	}
	used := empty.ask(t)
	if used.RetrievalStatus != models.RetrievalVectorUsed || !used.VectorUsed {
		t.Fatalf("a semantic hit = %q vector_used=%v", used.RetrievalStatus, used.VectorUsed)
	}

	broken := newAdviceFixture(t, true, `{"data":[]}`)
	broken.seedEvent(t, "e1", "开会")
	if got := broken.ask(t).RetrievalStatus; got != models.RetrievalFailed {
		t.Fatalf("a broken embedding endpoint = %q, want %q", got, models.RetrievalFailed)
	}
}

// An old answer must stop presenting itself as current once the records behind it
// change, and deleting a record counts as changing it.
func TestAdviceStalenessFollowsTheRecords(t *testing.T) {
	f := newAdviceFixture(t, false, embedOK)
	event := f.seedEvent(t, "e1", "他说下周给答复")
	session := f.ask(t)

	fresh, err := f.svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.SourceStale != 0 {
		t.Fatalf("a just-generated answer is not stale: %+v", fresh)
	}

	edited := *event
	edited.RawText = "他其实说的是下周再说"
	if err := f.events.Update(&edited); err != nil {
		t.Fatal(err)
	}
	stale, err := f.svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.SourceStale != 1 || stale.SourceStaleReason == "" {
		t.Fatalf("rewriting the evidence should mark the answer stale: %+v", stale)
	}

	if err := f.events.Delete("e1"); err != nil {
		t.Fatal(err)
	}
	deleted, err := f.svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.SourceStale != 1 {
		t.Fatal("deleting the evidence must leave the answer marked stale")
	}

	// The history path has to reach the same conclusion as the single read.
	list, err := f.svc.List(f.person.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].SourceStale != 1 {
		t.Fatalf("list staleness = %+v", list)
	}
}

// Adopting a strategy is where an answer becomes an action, so the item has to
// carry the words that were going to be used and point back at the reasoning.
func TestAdviceAdoptCreatesFollowUp(t *testing.T) {
	f := newAdviceFixture(t, false, embedOK)
	f.seedEvent(t, "e1", "他说下周给答复")
	session := f.ask(t)

	adopted, err := f.svc.Adopt(session.ID, models.AdoptRequest{StrategyIndex: 0, DueText: "这周内"})
	if err != nil {
		t.Fatal(err)
	}
	if adopted.AdoptedStrategyIndex == nil || *adopted.AdoptedStrategyIndex != 0 {
		t.Fatalf("adopted index = %v", adopted.AdoptedStrategyIndex)
	}
	if adopted.AdoptedStrategyName != "先共情再提事" {
		t.Fatalf("adopted name = %q", adopted.AdoptedStrategyName)
	}
	if len(adopted.FollowUpIDs) != 1 {
		t.Fatalf("follow-up ids = %v", adopted.FollowUpIDs)
	}

	followUp, err := f.followUps.Get(adopted.FollowUpIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if followUp.PersonID != f.person.ID || followUp.SourceAdviceID != session.ID {
		t.Fatalf("follow-up = %+v", followUp)
	}
	if followUp.Title != "先共情再提事" || followUp.Description != "最近是不是挺累的？" {
		t.Fatalf("the strategy's words were not carried over: %+v", followUp)
	}
	if followUp.DueText != "这周内" || followUp.Status != models.FollowUpPending {
		t.Fatalf("follow-up = %+v", followUp)
	}

	if _, err := f.svc.Adopt(session.ID, models.AdoptRequest{StrategyIndex: 9}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("an out-of-range strategy = %v, want ErrInvalidInput", err)
	}
	if _, err := f.svc.Adopt("ghost", models.AdoptRequest{}); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("adopting from a missing session = %v, want ErrNotFound", err)
	}
}

// Deleting the reasoning behind an action leaves a note on the action, not a
// silently detached row.
func TestAdviceDeleteMarksAdoptedFollowUp(t *testing.T) {
	f := newAdviceFixture(t, false, embedOK)
	f.seedEvent(t, "e1", "他说下周给答复")
	session := f.ask(t)
	adopted, err := f.svc.Adopt(session.ID, models.AdoptRequest{StrategyIndex: 1})
	if err != nil {
		t.Fatal(err)
	}
	followUpID := adopted.FollowUpIDs[0]

	if err := f.svc.Delete(session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Get(session.ID); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("the session should be gone, got %v", err)
	}

	followUp, err := f.followUps.Get(followUpID)
	if err != nil {
		t.Fatal("the follow-up must survive its advice being deleted")
	}
	if followUp.SourceAdviceStale != 1 || followUp.SourceAdviceStaleReason == "" {
		t.Fatalf("the item does not remember its advice is gone: %+v", followUp)
	}
	if followUp.SourceAdviceID != "" {
		t.Fatalf("the detached link should be cleared, got %q", followUp.SourceAdviceID)
	}
	if followUp.Status != models.FollowUpPending {
		t.Fatalf("deleting advice must not change the work itself: %+v", followUp)
	}

	if err := f.svc.Delete("ghost"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("deleting a missing session = %v, want ErrNotFound", err)
	}
}

func TestAdviceGenerateValidatesInput(t *testing.T) {
	f := newAdviceFixture(t, false, embedOK)
	ctx := context.Background()

	if _, err := f.svc.Generate(ctx, models.AdviceRequest{Question: "没有人物"}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("missing person = %v, want ErrInvalidInput", err)
	}
	if _, err := f.svc.Generate(ctx, models.AdviceRequest{PersonID: f.person.ID}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("missing question = %v, want ErrInvalidInput", err)
	}
	if _, err := f.svc.Generate(ctx, models.AdviceRequest{PersonID: "ghost", Question: "?"}); !errors.Is(err, models.ErrPersonNotFound) {
		t.Fatalf("unknown person = %v, want ErrPersonNotFound", err)
	}
}

// The mapping from conclusions to evidence is the promise this feature makes, so
// the filtering and the union are asserted directly.
func TestNormalizeAdviceEvidence(t *testing.T) {
	known := map[string]bool{"e1": true, "e2": true}
	advice := models.AdviceResponse{
		Situation:        models.AdvicePoint{EvidenceEventIDs: []string{"e1", "made-up"}},
		OtherPerspective: models.AdvicePoint{EvidenceEventIDs: nil},
		Risks:            []models.AdvicePoint{{EvidenceEventIDs: []string{"e2"}}},
		Strategies:       []models.Strategy{{EvidenceEventIDs: []string{"e1"}}, {}},
		FollowUp:         models.AdvicePoint{EvidenceEventIDs: []string{"made-up"}},
	}

	normalizeAdviceEvidence(&advice, known)

	if len(advice.Situation.EvidenceEventIDs) != 1 || advice.Situation.EvidenceEventIDs[0] != "e1" {
		t.Fatalf("situation evidence = %v", advice.Situation.EvidenceEventIDs)
	}
	if len(advice.EvidenceEventIDs) != 2 {
		t.Fatalf("union = %v, want e1 and e2 each once", advice.EvidenceEventIDs)
	}
	if advice.Risks == nil || advice.Strategies == nil {
		t.Fatal("nil sections must become empty lists so the JSON is an array")
	}
	if advice.OtherPerspective.EvidenceEventIDs == nil {
		t.Fatal("a conclusion with no evidence must still render as an empty array")
	}
	if advice.FollowUp.EvidenceEventIDs == nil || len(advice.FollowUp.EvidenceEventIDs) != 0 {
		t.Fatalf("an invented-only list must end up empty, got %#v", advice.FollowUp.EvidenceEventIDs)
	}
}
