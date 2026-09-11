package handler

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"relationship/internal/ai"
	"relationship/internal/config"
	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
	"relationship/internal/service"
)

type reportFixture struct {
	e         *echo.Echo
	followUps *repository.FollowUpRepo
	events    *repository.EventRepo
	person    *models.Person
}

// newReportAPI wires the report routes against a stubbed provider address. The
// endpoint is unreachable on purpose: generating with findings must survive a
// dead model, and an empty period never reaches for it at all.
func newReportAPI(t *testing.T) *reportFixture {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "report_handler_test.db"))
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
	followUpRepo := repository.NewFollowUpRepo(database)
	reportRepo := repository.NewReportRepo(database)

	persons := service.NewPersonService(personRepo, vec, repository.NewOrganizationRepo(database))
	person := &models.Person{ID: uuid.New().String(), Name: "张总"}
	if err := persons.Create(person); err != nil {
		t.Fatal(err)
	}

	cfg := &config.LLMConfig{Endpoint: "http://127.0.0.1:1", ExtractModel: "test", AdviceModel: "test-model"}
	aiService := service.NewAIService(cfg, vec, traitRepo, eventRepo, personRepo, ai.NewClient(cfg))
	reportService := service.NewReportService(
		reportRepo, eventRepo, followUpRepo,
		repository.NewRelationshipRepo(database), repository.NewPositionRepo(database),
		persons, aiService,
	)

	handler := NewReportHandler(reportService)
	e := echo.New()
	api := e.Group("/api")
	api.POST("/reports", handler.Generate)
	api.GET("/reports", handler.List)
	api.GET("/reports/:id", handler.Get)
	api.DELETE("/reports/:id", handler.Delete)

	return &reportFixture{e: e, followUps: followUpRepo, events: eventRepo, person: person}
}

// An empty POST is a valid request: the default window is the last seven days,
// and an empty period answers locally even with the model unreachable.
func TestReportAPIGenerateEmptyPeriod(t *testing.T) {
	f := newReportAPI(t)

	status, resp := doJSON(t, f.e, http.MethodPost, "/api/reports", "")
	if status != http.StatusCreated {
		t.Fatalf("generate returned %d: %+v", status, resp.Error)
	}
	raw, _ := json.Marshal(resp.Data)
	var report models.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != models.ReportSucceeded || report.GeneratedBy != "local" {
		t.Fatalf("report = %s by %s", report.Status, report.GeneratedBy)
	}
	if report.Overdue == nil || report.Events == nil || report.Persons == nil {
		t.Fatal("empty sections must render as arrays, not null")
	}
	if report.Days != 7 || report.Start == "" || report.End == "" {
		t.Fatalf("default window = %s ~ %s (%d days)", report.Start, report.End, report.Days)
	}

	// It is a snapshot: readable again, listed, deletable.
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/reports/"+report.ID, "")
	if status != http.StatusOK {
		t.Fatalf("read returned %d: %+v", status, resp.Error)
	}
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/reports?person_id="+f.person.ID, "")
	if status != http.StatusOK {
		t.Fatalf("list returned %d: %+v", status, resp.Error)
	}
	if rows, _ := resp.Data.([]any); len(rows) != 0 {
		t.Fatalf("a person-scoped history should not include the all-people report, got %d rows", len(rows))
	}
	status, _ = doJSON(t, f.e, http.MethodDelete, "/api/reports/"+report.ID, "")
	if status != http.StatusOK {
		t.Fatalf("delete returned %d", status)
	}
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/reports/"+report.ID, "")
	if status != http.StatusNotFound || resp.Error == nil || resp.Error.Code != "NOT_FOUND" {
		t.Fatalf("a deleted report = %d %+v, want 404 NOT_FOUND", status, resp.Error)
	}
}

// A period with findings against a dead model still answers 201 with every
// section complete and the failure marked — a 5xx would claim the caller got
// nothing when they got everything but the prose.
func TestReportAPIDegradedGenerationStillStores(t *testing.T) {
	f := newReportAPI(t)

	item := &models.FollowUp{
		ID: uuid.New().String(), PersonID: f.person.ID, Title: "追答复",
		DueDate: time.Now().AddDate(0, 0, -1).Format(models.DateLayout), Status: models.FollowUpPending,
	}
	if err := f.followUps.Create(item); err != nil {
		t.Fatal(err)
	}

	status, resp := doJSON(t, f.e, http.MethodPost, "/api/reports",
		`{"person_id":"`+f.person.ID+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("generate returned %d: %+v", status, resp.Error)
	}
	raw, _ := json.Marshal(resp.Data)
	var report models.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != models.ReportFailed || report.FailureReason == "" {
		t.Fatalf("status = %s reason = %q", report.Status, report.FailureReason)
	}
	if len(report.Overdue) != 1 || report.Overdue[0].Title != "追答复" || report.Overdue[0].DaysOverdue != 1 {
		t.Fatalf("overdue section = %+v", report.Overdue)
	}
	if len(report.Persons) != 1 || report.Persons[0].PersonName != "张总" {
		t.Fatalf("person summary = %+v", report.Persons)
	}

	// Validation errors surface as 400 / 404, not 500 from a provider round trip.
	status, resp = doJSON(t, f.e, http.MethodPost, "/api/reports", `{"week_of":"yesterday"}`)
	if status != http.StatusBadRequest || resp.Error == nil || resp.Error.Code != "INVALID_INPUT" {
		t.Fatalf("a bad week_of = %d %+v, want 400 INVALID_INPUT", status, resp.Error)
	}
	status, resp = doJSON(t, f.e, http.MethodPost, "/api/reports", `{"person_id":"ghost"}`)
	if status != http.StatusNotFound || resp.Error == nil || resp.Error.Code != "PERSON_NOT_FOUND" {
		t.Fatalf("an unknown person = %d %+v, want 404 PERSON_NOT_FOUND", status, resp.Error)
	}
}
