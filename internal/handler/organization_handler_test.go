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

type orgFixture struct {
	e       *echo.Echo
	persons *repository.PersonRepo
}

// organizationAPI wires the organisation routes against a throwaway database.
func organizationAPI(t *testing.T) *orgFixture {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "organization_handler_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	personRepo := repository.NewPersonRepo(database)
	orgRepo := repository.NewOrganizationRepo(database)
	if err := personRepo.Create(&models.Person{ID: uuid.New().String(), Name: "张总"}); err != nil {
		t.Fatal(err)
	}

	h := NewOrganizationHandler(service.NewOrganizationService(orgRepo))

	e := echo.New()
	api := e.Group("/api")
	api.POST("/organizations", h.Create)
	api.GET("/organizations", h.List)
	api.PUT("/organizations/:id", h.Update)
	api.POST("/organizations/:id/archive", h.Archive)
	api.POST("/organizations/:id/restore", h.Restore)
	api.DELETE("/organizations/:id", h.Delete)
	return &orgFixture{e: e, persons: personRepo}
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

func TestOrganizationValidation(t *testing.T) {
	f := organizationAPI(t)

	if status, _ := doJSON(t, f.e, http.MethodPost, "/api/organizations", `{"name":"   "}`); status != http.StatusBadRequest {
		t.Fatalf("blank name returned %d, want 400", status)
	}

	status, resp := doJSON(t, f.e, http.MethodPost, "/api/organizations", `{"name":"某某科技"}`)
	if status != http.StatusCreated {
		t.Fatalf("create returned %d", status)
	}
	if decodeOrg(t, resp).ID == "" {
		t.Fatal("created organization has no id")
	}
}

// Archive is the reversible path: the organisation leaves every picker, its row
// and its members survive, and restore brings it back.
func TestOrganizationArchiveHidesFromPickers(t *testing.T) {
	f := organizationAPI(t)

	status, resp := doJSON(t, f.e, http.MethodPost, "/api/organizations", `{"name":"某某科技"}`)
	if status != http.StatusCreated {
		t.Fatalf("create returned %d", status)
	}
	id := decodeOrg(t, resp).ID

	// Attach a person so the "archiving must not detach anyone" claim is real.
	persons, err := f.persons.ListWithActivity()
	if err != nil {
		t.Fatal(err)
	}
	person := &models.Person{ID: persons[0].ID, Name: persons[0].Name, OrgID: id}
	if err := f.persons.Update(person); err != nil {
		t.Fatal(err)
	}

	status, resp = doJSON(t, f.e, http.MethodPost, "/api/organizations/"+id+"/archive", "")
	if status != http.StatusOK {
		t.Fatalf("archive returned %d: %+v", status, resp.Error)
	}

	status, resp = doJSON(t, f.e, http.MethodGet, "/api/organizations", "")
	if status != http.StatusOK {
		t.Fatalf("list returned %d", status)
	}
	if orgs := decodeOrgs(t, resp); len(orgs) != 0 {
		t.Fatalf("an archived organization must not be offered as a picker, got %+v", orgs)
	}

	status, resp = doJSON(t, f.e, http.MethodGet, "/api/organizations?include_archived=true", "")
	if status != http.StatusOK {
		t.Fatalf("list with archived returned %d", status)
	}
	orgs := decodeOrgs(t, resp)
	if len(orgs) != 1 || orgs[0].ID != id || orgs[0].ArchivedAt == nil {
		t.Fatalf("the management list must still show it as archived, got %+v", orgs)
	}

	// The row survived, so the member keeps their employer.
	kept, err := f.persons.GetByID(person.ID)
	if err != nil {
		t.Fatal(err)
	}
	if kept.OrgID != id {
		t.Fatalf("archiving must not detach members, got org_id %q", kept.OrgID)
	}

	status, resp = doJSON(t, f.e, http.MethodPost, "/api/organizations/"+id+"/restore", "")
	if status != http.StatusOK {
		t.Fatalf("restore returned %d: %+v", status, resp.Error)
	}
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/organizations", "")
	if status != http.StatusOK {
		t.Fatalf("list returned %d", status)
	}
	if orgs := decodeOrgs(t, resp); len(orgs) != 1 || orgs[0].ArchivedAt != nil {
		t.Fatalf("a restored organization must be selectable again, got %+v", orgs)
	}

	// Unknown ids are a 404, not a silent success.
	if status, _ := doJSON(t, f.e, http.MethodPost, "/api/organizations/ghost/archive", ""); status != http.StatusNotFound {
		t.Fatalf("archiving an unknown organization returned %d, want 404", status)
	}
	if status, _ := doJSON(t, f.e, http.MethodPost, "/api/organizations/ghost/restore", ""); status != http.StatusNotFound {
		t.Fatalf("restoring an unknown organization returned %d, want 404", status)
	}
}
