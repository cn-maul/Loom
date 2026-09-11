package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
	"relationship/internal/service"
)

// followUpAPI wires the follow-up routes against a throwaway database and
// returns the person the fixtures hang off.
func followUpAPI(t *testing.T) (*echo.Echo, string) {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "follow_up_handler_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	persons := service.NewPersonService(
		repository.NewPersonRepo(database),
		repository.NewVecRepo(database),
		repository.NewOrganizationRepo(database),
	)
	person := &models.Person{ID: uuid.New().String(), Name: "张总"}
	if err := persons.Create(person); err != nil {
		t.Fatal(err)
	}

	h := NewFollowUpHandler(service.NewFollowUpService(
		repository.NewFollowUpRepo(database),
		persons,
		repository.NewEventRepo(database),
	))
	e := echo.New()
	api := e.Group("/api")
	api.POST("/follow-ups", h.Create)
	api.GET("/follow-ups", h.List)
	api.GET("/follow-ups/:id", h.Get)
	api.PUT("/follow-ups/:id", h.Update)
	api.GET("/follow-ups/:id/postponements", h.ListPostponements)
	api.POST("/follow-ups/:id/complete", h.Complete)
	api.POST("/follow-ups/:id/postpone", h.Postpone)
	api.POST("/follow-ups/:id/wait", h.Action("waiting"))
	api.POST("/follow-ups/:id/cancel", h.Action("cancelled"))
	api.GET("/persons/:id/follow-ups", h.ListByPerson)
	return e, person.ID
}

func doJSON(t *testing.T, e *echo.Echo, method, target, body string) (int, models.APIResponse) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var resp models.APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s %s returned unparsable body %q: %v", method, target, rec.Body.String(), err)
	}
	return rec.Code, resp
}

func createFollowUp(t *testing.T, e *echo.Echo, personID, body string) string {
	t.Helper()
	payload := `{"person_id":"` + personID + `","title":"跟进报价"` + body + `}`
	status, resp := doJSON(t, e, http.MethodPost, "/api/follow-ups", payload)
	if status != http.StatusCreated {
		t.Fatalf("create returned %d: %+v", status, resp.Error)
	}
	raw, _ := json.Marshal(resp.Data)
	var item models.FollowUp
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	return item.ID
}

func TestFollowUpAPICreateAndRead(t *testing.T) {
	e, personID := followUpAPI(t)
	id := createFollowUp(t, e, personID, "")

	status, resp := doJSON(t, e, http.MethodGet, "/api/follow-ups/"+id, "")
	if status != http.StatusOK {
		t.Fatalf("reading a pending follow-up returned %d: %+v", status, resp.Error)
	}

	// The per-person route must read the person id from its own path parameter.
	status, _ = doJSON(t, e, http.MethodGet, "/api/persons/"+personID+"/follow-ups", "")
	if status != http.StatusOK {
		t.Fatalf("listing by person returned %d", status)
	}
	status, resp = doJSON(t, e, http.MethodGet, "/api/persons/"+uuid.New().String()+"/follow-ups", "")
	if status != http.StatusOK {
		t.Fatalf("listing an unknown person's follow-ups returned %d", status)
	}
	items, _ := resp.Data.([]any)
	if len(items) != 0 {
		t.Fatalf("unknown person should have no follow-ups, got %d", len(items))
	}
}

func TestFollowUpAPIStatusClassification(t *testing.T) {
	e, personID := followUpAPI(t)
	id := createFollowUp(t, e, personID, "")
	ghost := uuid.New().String()

	cases := []struct {
		name   string
		method string
		target string
		body   string
		want   int
		code   string
	}{
		{"missing row", http.MethodGet, "/api/follow-ups/" + ghost, "", http.StatusNotFound, "NOT_FOUND"},
		{"unknown person", http.MethodPost, "/api/follow-ups",
			`{"person_id":"` + ghost + `","title":"T"}`, http.StatusNotFound, "PERSON_NOT_FOUND"},
		{"unknown source record", http.MethodPost, "/api/follow-ups",
			`{"person_id":"` + personID + `","title":"T","source_event_id":"` + ghost + `"}`, http.StatusNotFound, "NOT_FOUND"},
		{"unknown outcome record", http.MethodPost, "/api/follow-ups/" + id + "/complete",
			`{"completed_event_id":"` + ghost + `"}`, http.StatusNotFound, "NOT_FOUND"},
		{"history of missing row", http.MethodGet, "/api/follow-ups/" + ghost + "/postponements",
			"", http.StatusNotFound, "NOT_FOUND"},
		{"empty title", http.MethodPost, "/api/follow-ups",
			`{"person_id":"` + personID + `","title":"  "}`, http.StatusBadRequest, "INVALID_INPUT"},
		{"malformed deadline", http.MethodPost, "/api/follow-ups/" + id + "/postpone",
			`{"due_date":"2026/10/01"}`, http.StatusBadRequest, "INVALID_INPUT"},
		{"missing deadline", http.MethodPost, "/api/follow-ups/" + id + "/postpone",
			`{}`, http.StatusBadRequest, "INVALID_INPUT"},
		{"update missing row", http.MethodPut, "/api/follow-ups/" + ghost,
			`{"person_id":"` + personID + `","title":"T"}`, http.StatusNotFound, "NOT_FOUND"},
		{"complete missing row", http.MethodPost, "/api/follow-ups/" + ghost + "/complete",
			"", http.StatusNotFound, "NOT_FOUND"},
		{"bad date range", http.MethodGet, "/api/follow-ups?from=2026-10-01&to=2026-09-01",
			"", http.StatusBadRequest, "INVALID_INPUT"},
		{"unknown status filter", http.MethodGet, "/api/follow-ups?status=done",
			"", http.StatusBadRequest, "INVALID_INPUT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, resp := doJSON(t, e, tc.method, tc.target, tc.body)
			if status != tc.want {
				t.Fatalf("%s %s = %d, want %d (%+v)", tc.method, tc.target, status, tc.want, resp.Error)
			}
			if resp.Error == nil || resp.Error.Code != tc.code {
				t.Fatalf("error code = %+v, want %s", resp.Error, tc.code)
			}
		})
	}
}

func TestFollowUpAPIStateTransitions(t *testing.T) {
	e, personID := followUpAPI(t)
	id := createFollowUp(t, e, personID, `,"due_date":"2026-09-30"`)

	status, resp := doJSON(t, e, http.MethodPost, "/api/follow-ups/"+id+"/postpone", `{"due_date":"2026-10-01"}`)
	if status != http.StatusOK {
		t.Fatalf("postpone returned %d: %+v", status, resp.Error)
	}

	status, _ = doJSON(t, e, http.MethodPost, "/api/follow-ups/"+id+"/complete", "")
	if status != http.StatusOK {
		t.Fatalf("complete returned %d", status)
	}
	// Retrying a completion stays a success.
	status, _ = doJSON(t, e, http.MethodPost, "/api/follow-ups/"+id+"/complete", "")
	if status != http.StatusOK {
		t.Fatalf("repeated complete returned %d", status)
	}

	status, resp = doJSON(t, e, http.MethodPost, "/api/follow-ups/"+id+"/postpone", `{"due_date":"2026-11-01"}`)
	if status != http.StatusConflict {
		t.Fatalf("postponing a completed item returned %d, want 409 (%+v)", status, resp.Error)
	}

	status, _ = doJSON(t, e, http.MethodPost, "/api/follow-ups/"+id+"/cancel", "")
	if status != http.StatusConflict {
		t.Fatalf("cancelling a completed item returned %d, want 409", status)
	}
}

func decodeFollowUp(t *testing.T, resp models.APIResponse) models.FollowUp {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatal(err)
	}
	var item models.FollowUp
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	return item
}

// An item remembers where it came from and what came of it, through the API.
func TestFollowUpAPIClosureFields(t *testing.T) {
	e, personID := followUpAPI(t)

	status, resp := doJSON(t, e, http.MethodPost, "/api/follow-ups",
		`{"person_id":"`+personID+`","title":"跟进合同","owner":"我","due_text":"下周三前","due_date":"2026-09-30"}`)
	if status != http.StatusCreated {
		t.Fatalf("create returned %d: %+v", status, resp.Error)
	}
	created := decodeFollowUp(t, resp)
	if created.Owner != "我" || created.DueText != "下周三前" {
		t.Fatalf("owner / due_text did not round trip: %+v", created)
	}

	// A one-click completion stays possible, and may carry the outcome with it.
	status, _ = doJSON(t, e, http.MethodPost, "/api/follow-ups/"+created.ID+"/complete", "")
	if status != http.StatusOK {
		t.Fatalf("bare complete returned %d", status)
	}
	status, resp = doJSON(t, e, http.MethodPost, "/api/follow-ups/"+created.ID+"/complete",
		`{"completion_note":"张总已确认"}`)
	if status != http.StatusOK {
		t.Fatalf("complete with an outcome returned %d: %+v", status, resp.Error)
	}
	done := decodeFollowUp(t, resp)
	if done.Status != "completed" || done.CompletionNote != "张总已确认" {
		t.Fatalf("outcome not recorded: %+v", done)
	}

	// Waiting is a distinct state with its own action.
	other := createFollowUp(t, e, personID, "")
	status, resp = doJSON(t, e, http.MethodPost, "/api/follow-ups/"+other+"/wait", "")
	if status != http.StatusOK {
		t.Fatalf("wait returned %d: %+v", status, resp.Error)
	}
	if waiting := decodeFollowUp(t, resp); waiting.Status != "waiting" {
		t.Fatalf("status = %q, want waiting", waiting.Status)
	}
}

// Postponing keeps both the new deadline and the one it replaced.
func TestFollowUpAPIRecordsDeadlineHistory(t *testing.T) {
	e, personID := followUpAPI(t)
	id := createFollowUp(t, e, personID, `,"due_date":"2026-09-30"`)

	status, resp := doJSON(t, e, http.MethodPost, "/api/follow-ups/"+id+"/postpone",
		`{"due_date":"2026-10-01","reason":"对方出差"}`)
	if status != http.StatusOK {
		t.Fatalf("postpone returned %d: %+v", status, resp.Error)
	}
	if postponed := decodeFollowUp(t, resp); postponed.DueDate != "2026-10-01" {
		t.Fatalf("due date = %q", postponed.DueDate)
	}

	status, resp = doJSON(t, e, http.MethodGet, "/api/follow-ups/"+id+"/postponements", "")
	if status != http.StatusOK {
		t.Fatalf("history returned %d: %+v", status, resp.Error)
	}
	entries, _ := resp.Data.([]any)
	if len(entries) != 1 {
		t.Fatalf("want 1 history entry, got %d", len(entries))
	}
	entry, _ := entries[0].(map[string]any)
	if entry["reason"] != "对方出差" || entry["old_due_date"] != "2026-09-30" || entry["new_due_date"] != "2026-10-01" {
		t.Fatalf("history entry = %+v", entry)
	}

	// Reading the item carries the same history, so a detail page needs one call.
	_, resp = doJSON(t, e, http.MethodGet, "/api/follow-ups/"+id, "")
	if item := decodeFollowUp(t, resp); len(item.Postponements) != 1 {
		t.Fatalf("detail read returned %d history entries", len(item.Postponements))
	}
}
