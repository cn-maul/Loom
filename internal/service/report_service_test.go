package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"relationship/internal/ai"
	"relationship/internal/config"
	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
)

type reportFixture struct {
	svc       *ReportService
	database  *sql.DB
	events    *repository.EventRepo
	followUps *repository.FollowUpRepo
	relations *repository.RelationshipRepo
	person    *models.Person
	other     *models.Person

	mu         sync.Mutex
	lastPrompt string
}

// lastUserPrompt returns the most recent chat request body, so a test can
// assert on the outline that was sent to the model.
func (f *reportFixture) lastUserPrompt() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastPrompt
}

// newReportFixture stands up a stubbed provider. chatOK controls whether the
// narrative call succeeds, so the degraded path can be exercised without a
// second fixture type.
func newReportFixture(t *testing.T, chatOK bool) *reportFixture {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "report_service_test.db"))
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
	followUpRepo := repository.NewFollowUpRepo(database)
	relations := repository.NewRelationshipRepo(database)
	positions := repository.NewPositionRepo(database)
	reportRepo := repository.NewReportRepo(database)
	vec := repository.NewVecRepo(database)

	f := &reportFixture{
		database: database, events: eventRepo, followUps: followUpRepo,
		relations: relations, person: nil, other: nil,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/embeddings" {
			_, _ = w.Write([]byte(`{"data":[{"embedding":[1,0,0,0]}]}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.lastPrompt = string(body)
		f.mu.Unlock()
		if chatOK {
			_, _ = w.Write([]byte(`{"id":"c1","object":"chat.completion","model":"test","choices":[{"index":0,"message":{"role":"assistant","content":"这是模型写的周报叙述。"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			return
		}
		http.Error(w, "model down", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	cfg := &config.LLMConfig{
		Endpoint: server.URL, Protocol: "openai", APIKey: "test-key",
		ExtractModel: "test", AdviceModel: "test-model",
		EmbedModel: "test", EmbedDim: 4, MaxTokens: 2000,
	}
	persons := NewPersonService(personRepo, vec, repository.NewOrganizationRepo(database))
	person := &models.Person{ID: uuid.New().String(), Name: "张总"}
	other := &models.Person{ID: uuid.New().String(), Name: "李工"}
	if err := persons.Create(person); err != nil {
		t.Fatal(err)
	}
	if err := persons.Create(other); err != nil {
		t.Fatal(err)
	}
	f.person, f.other = person, other

	aiService := NewAIService(cfg, vec, traitRepo, eventRepo, personRepo, ai.NewClient(cfg))
	f.svc = NewReportService(reportRepo, eventRepo, followUpRepo, relations, positions, persons, aiService)
	return f
}

// today is the report window: the same seven days ending today the service
// defaults to, so bucket arithmetic against time.Now() stays deterministic.
func weekWindow() (string, string) {
	now := time.Now()
	return now.AddDate(0, 0, -6).Format(models.DateLayout), now.Format(models.DateLayout)
}

func (f *reportFixture) addFollowUp(t *testing.T, title, dueDate, status string) *models.FollowUp {
	t.Helper()
	item := &models.FollowUp{
		ID: uuid.New().String(), PersonID: f.person.ID, Title: title,
		DueDate: dueDate, Status: status,
	}
	if status == "" {
		item.Status = models.FollowUpPending
	}
	if err := f.followUps.Create(item); err != nil {
		t.Fatal(err)
	}
	return item
}

// backdate rewrites the creation stamp of one follow-up, which is how an item
// "has been open since before the period" without waiting for real weeks.
func (f *reportFixture) backdate(t *testing.T, id, createdStamp string) {
	t.Helper()
	if _, err := f.database.Exec(`UPDATE follow_ups SET created_at = ?, updated_at = ? WHERE id = ?`,
		createdStamp, createdStamp, id); err != nil {
		t.Fatal(err)
	}
}

// The buckets are the contract: disjoint, exhaustive over open work, and each
// item keeps the numbers a reader needs to act without opening it.
func TestReportBucketsCoverEveryOpenItem(t *testing.T) {
	f := newReportFixture(t, true)
	start, end := weekWindow()
	today := time.Now().Format(models.DateLayout)
	yesterday := time.Now().AddDate(0, 0, -1).Format(models.DateLayout)
	nextMonth := time.Now().AddDate(0, 0, 30).Format(models.DateLayout)

	overdue := f.addFollowUp(t, "追答复", yesterday, "")
	f.addFollowUp(t, "今天要发", today, "") // due inside the window → due_soon
	f.addFollowUp(t, "等对方回复", "", models.FollowUpWaiting)
	carried := f.addFollowUp(t, "上个月就开的事", "", "")
	f.backdate(t, carried.ID, time.Now().AddDate(0, 0, -30).UTC().Format("2006-01-02 15:04:05"))
	f.addFollowUp(t, "下个月的安排", nextMonth, "")
	f.addFollowUp(t, "已办完", "", models.FollowUpCompleted) // completed_at NULL falls back to updated_at = today
	cancelled := f.addFollowUp(t, "已取消", yesterday, models.FollowUpCancelled)

	report, err := f.svc.Generate(context.Background(), models.ReportRequest{Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Overdue) != 1 || report.Overdue[0].ID != overdue.ID {
		t.Fatalf("overdue = %+v", report.Overdue)
	}
	if report.Overdue[0].DaysOverdue != 1 {
		t.Fatalf("days overdue = %d, want 1", report.Overdue[0].DaysOverdue)
	}
	if len(report.DueSoon) != 1 || report.DueSoon[0].Title != "今天要发" {
		t.Fatalf("due_soon = %+v", report.DueSoon)
	}
	if len(report.Waiting) != 1 || report.Waiting[0].Title != "等对方回复" {
		t.Fatalf("waiting = %+v", report.Waiting)
	}
	if len(report.CarriedOver) != 1 || report.CarriedOver[0].ID != carried.ID {
		t.Fatalf("carried over = %+v", report.CarriedOver)
	}
	if len(report.Upcoming) != 1 || report.Upcoming[0].Title != "下个月的安排" {
		t.Fatalf("upcoming = %+v", report.Upcoming)
	}
	if len(report.Completed) != 1 || report.Completed[0].Title != "已办完" {
		t.Fatalf("completed = %+v", report.Completed)
	}
	// Cancelled work is not work: it must not appear in any bucket.
	for _, ref := range [][]models.ReportFollowUpRef{report.Overdue, report.DueSoon, report.Waiting, report.CarriedOver, report.Upcoming, report.Completed} {
		for _, item := range ref {
			if item.ID == cancelled.ID {
				t.Fatal("a cancelled item leaked into the report")
			}
		}
	}
	if report.OpenCount != 5 {
		t.Fatalf("open count = %d, want 5", report.OpenCount)
	}
	if report.GeneratedBy != "test-model" || report.Status != models.ReportSucceeded {
		t.Fatalf("a narrated report = %s by %s (%s)", report.Status, report.GeneratedBy, report.FailureReason)
	}
	if report.Summary != "这是模型写的周报叙述。" {
		t.Fatalf("the narrative did not replace the outline: %q", report.Summary)
	}
}

// The period's records, the promises inside them, and what moved on the
// relationship graph all have to land in the report with ids a reader can open.
func TestReportCollectsRecordsPromisesAndChanges(t *testing.T) {
	f := newReportFixture(t, true)
	start, end := weekWindow()
	today := time.Now().Format(models.DateLayout)

	event := &models.Event{
		ID: "e1", PersonID: f.person.ID, RawText: "聊了合作的事", EventDate: today,
		Promises: `[{"who":"张总","what":"下周给答复","deadline":"2026-09-15"}]`,
	}
	if err := f.events.Create(event); err != nil {
		t.Fatal(err)
	}
	link := &models.Relationship{
		ID: "r1", FromPersonID: f.person.ID, ToPersonID: f.other.ID,
		RelationType: "前同事", StartDate: today,
	}
	if err := link.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := f.relations.Create(link); err != nil {
		t.Fatal(err)
	}

	report, err := f.svc.Generate(context.Background(), models.ReportRequest{Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}

	if report.EventCount != 1 || len(report.Events) != 1 {
		t.Fatalf("events = %d (%d)", len(report.Events), report.EventCount)
	}
	got := report.Events[0]
	if got.ID != "e1" || got.PersonName != "张总" || got.Summary != "聊了合作的事" {
		t.Fatalf("event ref = %+v", got)
	}
	if len(report.Promises) != 1 || report.Promises[0].EventID != "e1" ||
		report.Promises[0].Who != "张总" || report.Promises[0].What != "下周给答复" {
		t.Fatalf("promises = %+v", report.Promises)
	}
	if len(report.Changes) != 1 {
		t.Fatalf("changes = %+v", report.Changes)
	}
	change := report.Changes[0]
	if change.Kind != models.ReportRelationshipStarted || change.PersonName != "张总" ||
		change.CounterpartName != "李工" || change.Description != "前同事" {
		t.Fatalf("change ref = %+v", change)
	}
	if len(report.Persons) != 1 || report.Persons[0].PersonName != "张总" ||
		report.Persons[0].EventCount != 1 || report.Persons[0].LastEventDate != today {
		t.Fatalf("person summary = %+v", report.Persons)
	}
	if report.Summary != "这是模型写的周报叙述。" || report.GeneratedBy != "test-model" {
		t.Fatalf("a successful narration = %q by %s", report.Summary, report.GeneratedBy)
	}
	// The outline that went to the model carries names, not raw ids.
	prompt := f.lastUserPrompt()
	if !contains(prompt, "张总") || contains(prompt, "e1") {
		t.Fatalf("outline sent to the model = %q", prompt)
	}
}

// A broken model costs the wording and nothing else: the structured report is
// stored, marked failed, and says why.
func TestReportNarrationFailureKeepsAggregation(t *testing.T) {
	f := newReportFixture(t, false)
	start, end := weekWindow()
	f.addFollowUp(t, "追答复", start, "")

	report, err := f.svc.Generate(context.Background(), models.ReportRequest{Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != models.ReportFailed || report.FailureReason == "" {
		t.Fatalf("status = %s reason = %q", report.Status, report.FailureReason)
	}
	if report.GeneratedBy != "local" {
		t.Fatalf("generated by = %q", report.GeneratedBy)
	}
	if !contains(report.Summary, "逾期事项") || !contains(report.Summary, "追答复") {
		t.Fatalf("the aggregation did not survive: %q", report.Summary)
	}

	// The degraded snapshot is readable again and the history marks it.
	stored, err := f.svc.Get(report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.ReportFailed || len(stored.Overdue) != 1 {
		t.Fatalf("stored = %s overdue=%d", stored.Status, len(stored.Overdue))
	}
	history, err := f.svc.List("", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].FailureReason == "" {
		t.Fatalf("history = %+v", history)
	}
}

// An empty period is answered without the model. Proving it with a broken
// endpoint makes the point: had the model been called, the status would fail.
func TestReportEmptyPeriodStaysLocal(t *testing.T) {
	f := newReportFixture(t, false)
	start, end := weekWindow()

	report, err := f.svc.Generate(context.Background(), models.ReportRequest{Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != models.ReportSucceeded || report.GeneratedBy != "local" {
		t.Fatalf("status = %s by %s (%s)", report.Status, report.GeneratedBy, report.FailureReason)
	}
	if !contains(report.Summary, "没有新的记录") {
		t.Fatalf("summary = %q", report.Summary)
	}
	if _, err := f.svc.Get(report.ID); err != nil {
		t.Fatal(err)
	}
}

func TestReportValidatesInput(t *testing.T) {
	f := newReportFixture(t, true)
	ctx := context.Background()

	if _, err := f.svc.Generate(ctx, models.ReportRequest{PersonID: "ghost"}); !errors.Is(err, models.ErrPersonNotFound) {
		t.Fatalf("unknown person = %v, want ErrPersonNotFound", err)
	}
	if _, err := f.svc.Generate(ctx, models.ReportRequest{Start: "2026-09-01"}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("half a window = %v, want ErrInvalidInput", err)
	}
	if _, err := f.svc.Generate(ctx, models.ReportRequest{Start: "2026-09-10", End: "2026-09-01"}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("inverted window = %v, want ErrInvalidInput", err)
	}
	if _, err := f.svc.Generate(ctx, models.ReportRequest{Start: "2025-01-01", End: "2026-09-01"}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("an oversized window = %v, want ErrInvalidInput", err)
	}
}

// One definition of "last week" lives in Resolve: week_of expands to the
// Monday–Sunday natural week containing the anchor date.
func TestReportRequestResolve(t *testing.T) {
	now := time.Date(2026, 9, 11, 15, 0, 0, 0, time.Local)

	req := models.ReportRequest{WeekOf: "2026-09-09"} // a Wednesday
	if err := req.Resolve(now); err != nil {
		t.Fatal(err)
	}
	if req.Start != "2026-09-07" || req.End != "2026-09-13" {
		t.Fatalf("week of 2026-09-09 = %s ~ %s", req.Start, req.End)
	}

	req = models.ReportRequest{}
	if err := req.Resolve(now); err != nil {
		t.Fatal(err)
	}
	if req.Start != "2026-09-05" || req.End != "2026-09-11" || req.DayCount() != 7 {
		t.Fatalf("default window = %s ~ %s (%d days)", req.Start, req.End, req.DayCount())
	}
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }
