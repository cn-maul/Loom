package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"

	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
	"relationship/internal/service"
)

// personListAPI wires the person list route against a throwaway database
// holding the same seed as the repository filter test.
func personListAPI(t *testing.T) *echo.Echo {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "person_list_handler_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO organizations (id, name) VALUES ('o1', '腾讯')`,
		`INSERT INTO persons (id, name, relation, importance, notes, created_at, updated_at)
		 VALUES ('p1', '陈静', '朋友', 5, '登山伙伴', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`,
		`INSERT INTO persons (id, name, relation, importance, org_id, position, created_at, updated_at)
		 VALUES ('p2', '张总', '同事', 3, 'o1', '总监', '2026-01-02 00:00:00', '2026-01-02 00:00:00')`,
		`INSERT INTO persons (id, name, relation, importance, org_id, position, created_at, updated_at)
		 VALUES ('p3', '李工', '同事', 2, 'o1', '工程师', '2026-01-03 00:00:00', '2026-01-03 00:00:00')`,
		`INSERT INTO persons (id, name, relation, importance, created_at, updated_at)
		 VALUES ('p4', '王姐', '家人', 1, '2026-01-04 00:00:00', '2026-01-04 00:00:00')`,
		`INSERT INTO events (id, person_id, event_date, raw_text, created_at)
		 VALUES ('e1', 'p2', '2026-09-01', '和张总开会', datetime('now'))`,
		`INSERT INTO events (id, person_id, event_date, raw_text, created_at)
		 VALUES ('e2', 'p3', '2026-09-10', '和李工吃饭', datetime('now'))`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	persons := service.NewPersonService(
		repository.NewPersonRepo(database),
		repository.NewVecRepo(database),
		repository.NewOrganizationRepo(database),
	)
	e := echo.New()
	e.GET("/api/persons", NewPersonHandler(persons).List)
	return e
}

func decodePersons(t *testing.T, resp models.APIResponse) []models.PersonWithActivity {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatal(err)
	}
	var persons []models.PersonWithActivity
	if err := json.Unmarshal(raw, &persons); err != nil {
		t.Fatalf("decode persons: %v", err)
	}
	return persons
}

func TestPersonListQueryFilters(t *testing.T) {
	e := personListAPI(t)

	// Keyword search reaches notes and position, not just names.
	status, resp := doJSON(t, e, http.MethodGet, "/api/persons?q=%E7%99%BB%E5%B1%B1", "")
	if status != http.StatusOK {
		t.Fatalf("search returned %d: %+v", status, resp.Error)
	}
	hits := decodePersons(t, resp)
	if len(hits) != 1 || hits[0].Name != "陈静" {
		t.Fatalf("notes search returned %+v", hits)
	}

	// The none sentinel selects persons without an affiliation and the list
	// still carries the joined org name for the rest.
	status, resp = doJSON(t, e, http.MethodGet, "/api/persons?org_id=none", "")
	if status != http.StatusOK {
		t.Fatalf("none filter returned %d: %+v", status, resp.Error)
	}
	hits = decodePersons(t, resp)
	if len(hits) != 2 {
		t.Fatalf("none filter returned %d persons", len(hits))
	}

	status, resp = doJSON(t, e, http.MethodGet, "/api/persons?org_id=o1", "")
	if status != http.StatusOK {
		t.Fatalf("org filter returned %d: %+v", status, resp.Error)
	}
	hits = decodePersons(t, resp)
	if len(hits) != 2 || hits[0].OrgName != "腾讯" {
		t.Fatalf("org filter returned %+v", hits)
	}
}

func TestPersonListPaginationCarriesTotal(t *testing.T) {
	e := personListAPI(t)

	// The page window comes back as data; the whole match count travels in
	// X-Total-Count so the caller knows a second page exists.
	req := httptest.NewRequest(http.MethodGet, "/api/persons?limit=2&offset=2", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("paged list returned %d", rec.Code)
	}
	if got := rec.Header().Get("X-Total-Count"); got != "4" {
		t.Fatalf("X-Total-Count = %q, want 4", got)
	}
	var resp models.APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	page := decodePersons(t, resp)
	if len(page) != 2 {
		t.Fatalf("second page holds %d persons, want 2", len(page))
	}
}

func TestPersonListRejectsUnknownSort(t *testing.T) {
	e := personListAPI(t)

	status, resp := doJSON(t, e, http.MethodGet, "/api/persons?sort=popularity", "")
	if status != http.StatusBadRequest {
		t.Fatalf("unknown sort returned %d, want 400", status)
	}
	if resp.Error == nil || resp.Error.Code != "INVALID_INPUT" {
		t.Fatalf("error = %+v, want INVALID_INPUT", resp.Error)
	}

	// Every documented ordering must be accepted, and a sort param of the
	// documented values must not fall into the 400 branch.
	for _, sort := range []string{models.PersonSortRecent, models.PersonSortName, models.PersonSortImportance, models.PersonSortCreated} {
		status, resp := doJSON(t, e, http.MethodGet, "/api/persons?sort="+sort, "")
		if status != http.StatusOK {
			t.Fatalf("sort=%s returned %d: %+v", sort, status, resp.Error)
		}
		if len(decodePersons(t, resp)) != 4 {
			t.Fatalf("sort=%s lost persons", sort)
		}
	}
}
