package handler

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
	"relationship/internal/service"
)

// organizationAPI wires the organisation routes against a throwaway database.
// Positions share the database so membership behaviour can be asserted too.
func organizationAPI(t *testing.T) *echo.Echo {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "organization_handler_test.db"))
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
	if err := persons.Create(&models.Person{ID: uuid.New().String(), Name: "张总"}); err != nil {
		t.Fatal(err)
	}

	orgRepo := repository.NewOrganizationRepo(database)
	orgService := service.NewOrganizationService(orgRepo)
	positionService := service.NewPositionService(repository.NewPositionRepo(database), persons, orgService)
	h := NewOrganizationHandler(orgService)
	ph := NewPositionHandler(positionService)

	e := echo.New()
	api := e.Group("/api")
	api.POST("/organizations", h.Create)
	api.GET("/organizations", h.List)
	api.PUT("/organizations/:id", h.Update)
	api.POST("/organizations/:id/archive", h.Archive)
	api.POST("/organizations/:id/restore", h.Restore)
	api.DELETE("/organizations/:id", h.Delete)
	api.POST("/persons/:id/positions", ph.Create)
	api.GET("/organizations/:id/members", ph.ListByOrg)
	api.PUT("/positions/:id", ph.Update)
	return e
}

func decodeOrgs(t *testing.T, resp models.APIResponse) []models.Organization {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatal(err)
	}
	var orgs []models.Organization
	if err := json.Unmarshal(raw, &orgs); err != nil {
		t.Fatalf("decode organizations: %v", err)
	}
	return orgs
}

func decodeOrg(t *testing.T, resp models.APIResponse) models.Organization {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatal(err)
	}
	var org models.Organization
	if err := json.Unmarshal(raw, &org); err != nil {
		t.Fatalf("decode organization: %v", err)
	}
	return org
}

func TestOrganizationArchiveHidesFromPickers(t *testing.T) {
	e := organizationAPI(t)

	status, resp := doJSON(t, e, http.MethodPost, "/api/organizations", `{"name":"某某科技","kind":"公司","description":"做社交 app"}`)
	if status != http.StatusCreated {
		t.Fatalf("create returned %d: %+v", status, resp.Error)
	}
	created := decodeOrg(t, resp)
	if created.Kind != "公司" || created.Description != "做社交 app" {
		t.Fatalf("created = %+v", created)
	}
	id := created.ID

	// Empty description round-trips as an empty string, never the string "null".
	status, resp = doJSON(t, e, http.MethodPost, "/api/organizations", `{"name":"无名协会"}`)
	if status != http.StatusCreated {
		t.Fatalf("create minimal returned %d: %+v", status, resp.Error)
	}
	if org := decodeOrg(t, resp); org.Description != "" {
		t.Fatalf("empty description became %q", org.Description)
	}

	status, resp = doJSON(t, e, http.MethodPost, "/api/organizations/"+id+"/archive", "")
	if status != http.StatusOK {
		t.Fatalf("archive returned %d: %+v", status, resp.Error)
	}
	if archived := decodeOrg(t, resp); archived.ArchivedAt == nil {
		t.Fatalf("archived org must carry a timestamp: %+v", archived)
	}

	// Default listing is for pickers: archived must be gone.
	status, resp = doJSON(t, e, http.MethodGet, "/api/organizations", "")
	if status != http.StatusOK {
		t.Fatalf("list returned %d", status)
	}
	if list := decodeOrgs(t, resp); len(list) != 1 || list[0].ID == id {
		t.Fatalf("active list should hide the archived org: %+v", list)
	}

	// The management page asks for everything and separates the states itself.
	status, resp = doJSON(t, e, http.MethodGet, "/api/organizations?include_archived=true", "")
	if status != http.StatusOK {
		t.Fatalf("list all returned %d", status)
	}
	if list := decodeOrgs(t, resp); len(list) != 2 {
		t.Fatalf("include_archived list = %d orgs, want 2", len(list))
	}

	status, resp = doJSON(t, e, http.MethodPost, "/api/organizations/"+id+"/restore", "")
	if status != http.StatusOK {
		t.Fatalf("restore returned %d: %+v", status, resp.Error)
	}
	if restored := decodeOrg(t, resp); restored.ArchivedAt != nil {
		t.Fatalf("restored org must lose the timestamp: %+v", restored)
	}
	status, resp = doJSON(t, e, http.MethodGet, "/api/organizations", "")
	if list := decodeOrgs(t, resp); len(list) != 2 {
		t.Fatalf("after restore the active list should hold both, got %d", len(list))
	}

	if status, resp := doJSON(t, e, http.MethodPost, "/api/organizations/ghost/archive", ""); status != http.StatusNotFound {
		t.Fatalf("archiving an unknown org returned %d: %+v", status, resp.Error)
	}
}

func TestOrganizationValidationAndMembers(t *testing.T) {
	e := organizationAPI(t)

	if status, _ := doJSON(t, e, http.MethodPost, "/api/organizations", `{"name":"   "}`); status != http.StatusBadRequest {
		t.Fatalf("blank name returned %d, want 400", status)
	}

	status, resp := doJSON(t, e, http.MethodPost, "/api/organizations", `{"name":"某某科技"}`)
	if status != http.StatusCreated {
		t.Fatalf("create returned %d", status)
	}
	id := decodeOrg(t, resp).ID

	if status, _ := doJSON(t, e, http.MethodGet, "/api/organizations/"+id+"/members", ""); status != http.StatusOK {
		t.Fatalf("members of a new org returned %d", status)
	}
	if status, _ := doJSON(t, e, http.MethodGet, "/api/organizations/ghost/members", ""); status != http.StatusNotFound {
		t.Fatalf("members of an unknown org returned %d, want 404", status)
	}

	// Membership flows (add / end stint / delete) are covered by the end-to-end
	// smoke against the real binary; the positions PUT contract is pinned by
	// position service tests.
}
