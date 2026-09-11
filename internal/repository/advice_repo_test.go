package repository

import (
	"errors"
	"testing"

	"relationship/internal/models"
)

func seedAdviceSession(t *testing.T, repo *AdviceRepo, id, personID, question string) *models.AdviceSession {
	t.Helper()
	session := &models.AdviceSession{
		ID:       id,
		PersonID: personID,
		Question: question,
		Goal:     "把话说清楚但别翻脸",
		AdviceResponse: models.AdviceResponse{
			Situation: models.AdvicePoint{
				Text:             "他最近在回避这个话题",
				EvidenceEventIDs: []string{"e1"},
			},
			OtherPerspective: models.AdvicePoint{Text: "他可能也在等一个台阶"},
			Risks: []models.AdvicePoint{
				{Text: "把话说硬了会伤关系", EvidenceEventIDs: []string{"e1"}},
			},
			Strategies: []models.Strategy{
				{
					Name: "先共情再提事", Script: "最近是不是挺累的？", Pros: "降低对抗", Cons: "见效慢",
					EvidenceEventIDs: []string{"e1"},
				},
				{Name: "给一个具体选项", Script: "要不我们约周三？", Pros: "直接", Cons: "可能被拒"},
			},
			FollowUp:         models.AdvicePoint{Text: "三天后再确认一次"},
			EvidenceEventIDs: []string{"e1"},
			VectorUsed:       true,
			RetrievalStatus:  models.RetrievalVectorUsed,
		},
		UsedEventIDs:    []string{"e1"},
		UsedTraitIDs:    []string{"t1"},
		EvidenceVersion: []models.EvidenceVersion{{EventID: "e1", ContentRev: "rev1"}},
		Model:           "test-model",
		FollowUpIDs:     []string{},
	}
	if err := repo.Create(session); err != nil {
		t.Fatal(err)
	}
	return session
}

// A stored answer has to come back with its structure intact: the per-conclusion
// evidence is the whole reason the session is persisted, so losing it on the way
// back would defeat the feature.
func TestAdviceRepoRoundTripsSession(t *testing.T) {
	database := newTestDB(t)
	repo := NewAdviceRepo(database)
	seedAdviceSession(t, repo, "a1", "p1", "怎么开口")

	stored, err := repo.Get("a1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Question != "怎么开口" || stored.Goal != "把话说清楚但别翻脸" {
		t.Fatalf("question/goal = %q / %q", stored.Question, stored.Goal)
	}
	if stored.Situation.Text != "他最近在回避这个话题" || len(stored.Situation.EvidenceEventIDs) != 1 {
		t.Fatalf("situation lost its evidence: %+v", stored.Situation)
	}
	if len(stored.Risks) != 1 || len(stored.Risks[0].EvidenceEventIDs) != 1 {
		t.Fatalf("risks lost their evidence: %+v", stored.Risks)
	}
	if len(stored.Strategies) != 2 {
		t.Fatalf("strategies = %d, want 2", len(stored.Strategies))
	}
	if len(stored.Strategies[0].EvidenceEventIDs) != 1 {
		t.Fatal("the first strategy lost its evidence")
	}
	if len(stored.Strategies[1].EvidenceEventIDs) != 0 {
		t.Fatal("a strategy with no evidence must stay empty rather than inherit one")
	}
	if stored.RetrievalStatus != models.RetrievalVectorUsed || !stored.VectorUsed {
		t.Fatalf("retrieval status = %q vector_used=%v", stored.RetrievalStatus, stored.VectorUsed)
	}
	if stored.Model != "test-model" {
		t.Fatalf("model = %q", stored.Model)
	}
	if len(stored.EvidenceVersion) != 1 || stored.EvidenceVersion[0].ContentRev != "rev1" {
		t.Fatalf("evidence versions = %+v", stored.EvidenceVersion)
	}
	if stored.AdoptedStrategyIndex != nil {
		t.Fatal("nothing has been adopted yet")
	}
	if stored.FollowUpIDs == nil || len(stored.FollowUpIDs) != 0 {
		t.Fatalf("follow-up ids should be an empty list, got %#v", stored.FollowUpIDs)
	}
}

func TestAdviceRepoGetMissing(t *testing.T) {
	database := newTestDB(t)
	repo := NewAdviceRepo(database)
	if _, err := repo.Get("ghost"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("Get(ghost) = %v, want ErrNotFound", err)
	}
}

// The history is read newest first and can be narrowed to one person; a tie on
// the second-resolution timestamp must still come back in a stable order.
func TestAdviceRepoListsPerPersonNewestFirst(t *testing.T) {
	database := newTestDB(t)
	if _, err := database.Exec(`INSERT INTO persons (id, name) VALUES ('p2', '李工')`); err != nil {
		t.Fatal(err)
	}
	repo := NewAdviceRepo(database)
	seedAdviceSession(t, repo, "a1", "p1", "第一个问题")
	seedAdviceSession(t, repo, "a2", "p1", "第二个问题")
	seedAdviceSession(t, repo, "a3", "p2", "别人的问题")

	list, err := repo.List("p1", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("p1 history = %d rows, want 2", len(list))
	}
	if list[0].ID != "a2" {
		t.Fatalf("newest first expected a2 at the head, got %s", list[0].ID)
	}

	all, err := repo.List("", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("unfiltered history = %d rows, want 3", len(all))
	}

	paged, err := repo.List("p1", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(paged) != 1 || paged[0].ID != "a1" {
		t.Fatalf("second page = %+v", paged)
	}
}

func TestAdviceRepoRecordsAdoptedStrategy(t *testing.T) {
	database := newTestDB(t)
	repo := NewAdviceRepo(database)
	seedAdviceSession(t, repo, "a1", "p1", "怎么开口")

	if err := repo.SetAdopted("a1", 1, "给一个具体选项"); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Get("a1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.AdoptedStrategyIndex == nil || *stored.AdoptedStrategyIndex != 1 {
		t.Fatalf("adopted index = %v", stored.AdoptedStrategyIndex)
	}
	if stored.AdoptedStrategyName != "给一个具体选项" {
		t.Fatalf("adopted name = %q", stored.AdoptedStrategyName)
	}

	if err := repo.SetAdopted("ghost", 0, "x"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("adopting on a missing session = %v, want ErrNotFound", err)
	}
	if err := repo.Delete("ghost"); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("deleting a missing session = %v, want ErrNotFound", err)
	}
}

// The follow-up keeps a marker when the reasoning behind it disappears, and the
// batch lookup that surfaces the link must not miss it.
func TestFollowUpRepoMarksAdviceSourceStale(t *testing.T) {
	database := newTestDB(t)
	adviceRepo := NewAdviceRepo(database)
	followUpRepo := NewFollowUpRepo(database)
	seedAdviceSession(t, adviceRepo, "a1", "p1", "怎么开口")

	for _, id := range []string{"f1", "f2"} {
		if err := followUpRepo.Create(&models.FollowUp{
			ID: id, PersonID: "p1", Title: "约张总喝咖啡", Status: models.FollowUpPending,
			SourceAdviceID: "a1",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// An item typed by hand must not be touched by an advice going away.
	if err := followUpRepo.Create(&models.FollowUp{
		ID: "f3", PersonID: "p1", Title: "手写事项", Status: models.FollowUpPending,
	}); err != nil {
		t.Fatal(err)
	}

	grouped, err := followUpRepo.ListIDsByAdvice([]string{"a1", "a-none"})
	if err != nil {
		t.Fatal(err)
	}
	if len(grouped["a1"]) != 2 {
		t.Fatalf("follow-ups linked to a1 = %v", grouped["a1"])
	}
	if ids, ok := grouped["a-none"]; !ok || len(ids) != 0 {
		t.Fatalf("a session with no follow-ups must still be present and empty, got %#v", grouped["a-none"])
	}

	affected, err := followUpRepo.MarkSourceAdviceStale("a1", "来源建议已删除，依据需要重新评估")
	if err != nil {
		t.Fatal(err)
	}
	if affected != 2 {
		t.Fatalf("marking affected %d follow-ups, want 2", affected)
	}

	stored, err := followUpRepo.Get("f1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.SourceAdviceStale != 1 || stored.SourceAdviceStaleReason == "" {
		t.Fatalf("the item does not remember its advice is gone: %+v", stored)
	}
	untouched, err := followUpRepo.Get("f3")
	if err != nil {
		t.Fatal(err)
	}
	if untouched.SourceAdviceStale != 0 || untouched.SourceAdviceID != "" {
		t.Fatalf("a hand-written item must be untouched: %+v", untouched)
	}

	// A second pass must not overwrite the original explanation.
	again, err := followUpRepo.MarkSourceAdviceStale("a1", "另一个原因")
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("a second marking changed %d already-stale rows", again)
	}
}

// Content fingerprints are what make staleness a comparison rather than a guess.
func TestEventRepoContentRevsTrackEdits(t *testing.T) {
	database := newTestDB(t)
	repo := NewEventRepo(database)
	insertEventRow(t, database, "e1", "p1")
	insertEventRow(t, database, "e2", "p1")

	before, err := repo.ContentRevs([]string{"e1", "e2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 || before["e1"] == "" {
		t.Fatalf("revisions = %#v", before)
	}

	event, err := repo.GetByID("e1")
	if err != nil {
		t.Fatal(err)
	}
	event.RawText = "改过的原文"
	if err := repo.Update(event); err != nil {
		t.Fatal(err)
	}

	after, err := repo.ContentRevs([]string{"e1", "e2"})
	if err != nil {
		t.Fatal(err)
	}
	if after["e1"] == before["e1"] {
		t.Fatal("rewriting a record must change its fingerprint")
	}
	if after["e2"] != before["e2"] {
		t.Fatal("an untouched record must keep its fingerprint")
	}

	// A deleted record has to disappear from the map: that absence is how the
	// caller tells deletion apart from an empty result.
	if err := repo.Delete("e2"); err != nil {
		t.Fatal(err)
	}
	gone, err := repo.ContentRevs([]string{"e1", "e2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gone["e2"]; ok {
		t.Fatal("a deleted record must not report a revision")
	}

	empty, err := repo.ContentRevs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("no ids should mean no work, got %#v", empty)
	}
}
