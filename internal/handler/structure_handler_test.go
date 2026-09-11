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

type structureFixture struct {
	e       *echo.Echo
	persons []*models.Person
	org     *models.Organization
	eventID string
}

// newStructureAPI wires relationships, postings and record participants against a
// throwaway database, so these tests never touch data/ or any developer database.
func newStructureAPI(t *testing.T) *structureFixture {
	t.Helper()
	database, err := db.OpenDB(filepath.Join(t.TempDir(), "structure_handler_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database); err != nil {
		t.Fatal(err)
	}

	personService := service.NewPersonService(
		repository.NewPersonRepo(database),
		repository.NewVecRepo(database),
		repository.NewOrganizationRepo(database),
	)
	orgService := service.NewOrganizationService(repository.NewOrganizationRepo(database))
	eventService := service.NewEventService(
		repository.NewEventRepo(database),
		repository.NewVecRepo(database),
		personService,
		repository.NewTraitRepo(database),
	)
	relService := service.NewRelationshipService(repository.NewRelationshipRepo(database), personService, eventService)
	posService := service.NewPositionService(repository.NewPositionRepo(database), personService, orgService)

	org := &models.Organization{ID: uuid.New().String(), Name: "腾讯"}
	if err := orgService.Create(org); err != nil {
		t.Fatal(err)
	}
	var persons []*models.Person
	for _, name := range []string{"张总", "李工", "王姐"} {
		p := &models.Person{ID: uuid.New().String(), Name: name}
		if err := personService.Create(p); err != nil {
			t.Fatal(err)
		}
		persons = append(persons, p)
	}

	// A record with two attendees, written through the repository so the fixture
	// does not need a language model.
	event := &models.Event{
		ID: uuid.New().String(), PersonID: persons[0].ID, RawText: "会议", EventDate: "2026-09-02",
		Participants: []models.EventParticipant{{PersonID: persons[0].ID}, {PersonID: persons[1].ID, Role: "同事"}},
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewEventRepo(database).Create(event); err != nil {
		t.Fatal(err)
	}

	relHandler := NewRelationshipHandler(relService)
	posHandler := NewPositionHandler(posService)
	eventHandler := NewEventHandler(eventService, nil)

	e := echo.New()
	api := e.Group("/api")
	api.POST("/relationships", relHandler.Create)
	api.GET("/relationships", relHandler.List)
	api.GET("/relationships/types", relHandler.ListTypes)
	api.GET("/relationships/co-attendance", relHandler.CoAttendance)
	api.GET("/relationships/:id", relHandler.Get)
	api.PUT("/relationships/:id", relHandler.Update)
	api.DELETE("/relationships/:id", relHandler.Delete)
	api.GET("/persons/:id/relationships", relHandler.ListByPerson)
	api.POST("/persons/:id/positions", posHandler.Create)
	api.GET("/persons/:id/positions", posHandler.ListByPerson)
	api.GET("/organizations/:id/members", posHandler.ListByOrg)
	api.PUT("/positions/:id", posHandler.Update)
	api.DELETE("/positions/:id", posHandler.Delete)
	api.GET("/events/:id", eventHandler.GetByID)
	api.GET("/events/:id/participants", eventHandler.ListParticipants)
	api.PUT("/events/:id/participants", eventHandler.SetParticipants)
	api.DELETE("/events/:id/participants/:personID", eventHandler.RemoveParticipant)

	return &structureFixture{e: e, persons: persons, org: org, eventID: event.ID}
}

func (f *structureFixture) createRelationship(t *testing.T, body string) string {
	t.Helper()
	status, resp := doJSON(t, f.e, http.MethodPost, "/api/relationships", body)
	if status != http.StatusCreated {
		t.Fatalf("create relationship returned %d: %+v", status, resp.Error)
	}
	raw, _ := json.Marshal(resp.Data)
	var link models.RelationshipLink
	if err := json.Unmarshal(raw, &link); err != nil {
		t.Fatal(err)
	}
	if link.FromPersonName == "" || link.ToPersonName == "" {
		t.Fatalf("created edge is missing endpoint names: %+v", link)
	}
	return link.ID
}

func (f *structureFixture) relationshipPayload(extra string) string {
	return `{"from_person_id":"` + f.persons[0].ID + `","to_person_id":"` + f.persons[1].ID +
		`","relation_type":"上级","confirmed":1` + extra + `}`
}

// Closing an edge sets an end_date and keeps it in the history.
func TestRelationshipAPICreateThenClose(t *testing.T) {
	f := newStructureAPI(t)
	id := f.createRelationship(t, f.relationshipPayload(""))

	status, resp := doJSON(t, f.e, http.MethodPut, "/api/relationships/"+id,
		`{"from_person_id":"`+f.persons[0].ID+`","to_person_id":"`+f.persons[1].ID+`","relation_type":"上级","end_date":"2026-08-31"}`)
	if status != http.StatusOK {
		t.Fatalf("closing the edge returned %d: %+v", status, resp.Error)
	}

	status, resp = doJSON(t, f.e, http.MethodGet, "/api/relationships?active=true", "")
	if status != http.StatusOK {
		t.Fatalf("active filter returned %d", status)
	}
	if items, _ := resp.Data.([]any); len(items) != 0 {
		t.Fatalf("a closed edge must not count as active, got %d", len(items))
	}
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/relationships", "")
	if status != http.StatusOK {
		t.Fatalf("listing all edges returned %d", status)
	}
	if items, _ := resp.Data.([]any); len(items) != 1 {
		t.Fatal("a closed edge must still be listed as history")
	}

	// The static routes must win over /relationships/:id.
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/relationships/types", "")
	if status != http.StatusOK {
		t.Fatalf("types returned %d", status)
	}
	if types, _ := resp.Data.([]any); len(types) != 1 || types[0] != "上级" {
		t.Fatalf("types = %+v", types)
	}
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/relationships/co-attendance", "")
	if status != http.StatusOK {
		t.Fatalf("co-attendance returned %d", status)
	}
	if pairs, _ := resp.Data.([]any); len(pairs) != 1 {
		t.Fatalf("the shared record should produce one pair, got %d", len(pairs))
	}
}

func TestStructureAPIStatusClassification(t *testing.T) {
	f := newStructureAPI(t)
	ghost := uuid.New().String()
	edge := f.createRelationship(t, f.relationshipPayload(""))

	cases := []struct {
		name   string
		method string
		target string
		body   string
		want   int
		code   string
	}{
		{"self relationship", http.MethodPost, "/api/relationships",
			`{"from_person_id":"` + f.persons[0].ID + `","to_person_id":"` + f.persons[0].ID + `","relation_type":"本人"}`,
			http.StatusBadRequest, "INVALID_INPUT"},
		{"missing type", http.MethodPost, "/api/relationships",
			`{"from_person_id":"` + f.persons[0].ID + `","to_person_id":"` + f.persons[1].ID + `"}`,
			http.StatusBadRequest, "INVALID_INPUT"},
		{"unknown person", http.MethodPost, "/api/relationships",
			`{"from_person_id":"` + ghost + `","to_person_id":"` + f.persons[1].ID + `","relation_type":"上级"}`,
			http.StatusNotFound, "PERSON_NOT_FOUND"},
		{"unknown source event", http.MethodPost, "/api/relationships",
			`{"from_person_id":"` + f.persons[0].ID + `","to_person_id":"` + f.persons[1].ID + `","relation_type":"上级","source_event_id":"` + ghost + `"}`,
			http.StatusNotFound, "NOT_FOUND"},
		{"backwards date window", http.MethodPost, "/api/relationships",
			`{"from_person_id":"` + f.persons[0].ID + `","to_person_id":"` + f.persons[1].ID + `","relation_type":"上级","start_date":"2026-09-01","end_date":"2026-08-01"}`,
			http.StatusBadRequest, "INVALID_INPUT"},
		{"unknown edge", http.MethodGet, "/api/relationships/" + ghost, "", http.StatusNotFound, "NOT_FOUND"},
		{"update unknown edge", http.MethodPut, "/api/relationships/" + ghost,
			`{"from_person_id":"` + f.persons[0].ID + `","to_person_id":"` + f.persons[1].ID + `","relation_type":"上级"}`,
			http.StatusNotFound, "NOT_FOUND"},
		{"delete unknown edge", http.MethodDelete, "/api/relationships/" + ghost, "", http.StatusNotFound, "NOT_FOUND"},
		{"bad confirmed filter", http.MethodGet, "/api/relationships?confirmed=7", "", http.StatusBadRequest, "INVALID_INPUT"},
		{"bad active filter", http.MethodGet, "/api/relationships?active=maybe", "", http.StatusBadRequest, "INVALID_INPUT"},
		{"unknown org members", http.MethodGet, "/api/organizations/" + ghost + "/members", "", http.StatusNotFound, "ORG_NOT_FOUND"},
		{"posting at unknown org", http.MethodPost, "/api/persons/" + f.persons[0].ID + "/positions",
			`{"org_id":"` + ghost + `","role":"总监"}`, http.StatusNotFound, "ORG_NOT_FOUND"},
		{"posting for unknown person", http.MethodPost, "/api/persons/" + ghost + "/positions",
			`{"org_id":"` + f.org.ID + `","role":"总监"}`, http.StatusNotFound, "PERSON_NOT_FOUND"},
		{"posting person mismatch", http.MethodPost, "/api/persons/" + f.persons[0].ID + "/positions",
			`{"person_id":"` + f.persons[1].ID + `","org_id":"` + f.org.ID + `"}`, http.StatusBadRequest, "INVALID_INPUT"},
		{"participants of unknown record", http.MethodGet, "/api/events/" + ghost + "/participants", "", http.StatusNotFound, "NOT_FOUND"},
		{"empty attendance list", http.MethodPut, "/api/events/" + f.eventID + "/participants",
			`{"participants":[]}`, http.StatusBadRequest, "INVALID_INPUT"},
		{"participant is not attending", http.MethodDelete, "/api/events/" + f.eventID + "/participants/" + ghost,
			"", http.StatusNotFound, "NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, resp := doJSON(t, f.e, tc.method, tc.target, tc.body)
			if status != tc.want {
				t.Fatalf("%s %s = %d, want %d (%+v)", tc.method, tc.target, status, tc.want, resp.Error)
			}
			if resp.Error == nil || resp.Error.Code != tc.code {
				t.Fatalf("error code = %+v, want %s", resp.Error, tc.code)
			}
		})
	}

	// The edge created before the table still exists after all the failures.
	status, _ := doJSON(t, f.e, http.MethodGet, "/api/relationships/"+edge, "")
	if status != http.StatusOK {
		t.Fatalf("the seeded edge disappeared, GET returned %d", status)
	}
}

// Posting history is readable per person and per organisation, and a posting is
// closed by setting an end_date rather than deleting it.
func TestPositionAPIHistoryAndMembers(t *testing.T) {
	f := newStructureAPI(t)
	personID := f.persons[0].ID

	status, resp := doJSON(t, f.e, http.MethodPost, "/api/persons/"+personID+"/positions",
		`{"org_id":"`+f.org.ID+`","role":"总监"}`)
	if status != http.StatusCreated {
		t.Fatalf("create posting returned %d: %+v", status, resp.Error)
	}
	raw, _ := json.Marshal(resp.Data)
	var posting models.OrgPositionLink
	if err := json.Unmarshal(raw, &posting); err != nil {
		t.Fatal(err)
	}
	if posting.PersonName != "张总" || posting.OrgName != "腾讯" {
		t.Fatalf("posting names were not resolved: %+v", posting)
	}

	// The person appears in the current member list, then moves to the history.
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/organizations/"+f.org.ID+"/members?current=true", "")
	if status != http.StatusOK {
		t.Fatalf("current members returned %d", status)
	}
	if members, _ := resp.Data.([]any); len(members) != 1 {
		t.Fatalf("want 1 current member, got %d", len(members))
	}

	status, resp = doJSON(t, f.e, http.MethodPut, "/api/positions/"+posting.ID,
		`{"person_id":"`+personID+`","org_id":"`+f.org.ID+`","role":"总监","end_date":"2026-08-31"}`)
	if status != http.StatusOK {
		t.Fatalf("closing the posting returned %d: %+v", status, resp.Error)
	}
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/organizations/"+f.org.ID+"/members?current=true", "")
	if status != http.StatusOK {
		t.Fatalf("current members returned %d", status)
	}
	if members, _ := resp.Data.([]any); len(members) != 0 {
		t.Fatal("an ended posting must leave the current member list")
	}
	status, resp = doJSON(t, f.e, http.MethodGet, "/api/persons/"+personID+"/positions", "")
	if status != http.StatusOK {
		t.Fatalf("posting history returned %d", status)
	}
	if history, _ := resp.Data.([]any); len(history) != 1 {
		t.Fatal("an ended posting must remain in the person's history")
	}
}

// A shared record is stored once and stays readable from every attendee, and the
// last participant cannot be removed.
func TestParticipantAPISharedRecord(t *testing.T) {
	f := newStructureAPI(t)

	status, resp := doJSON(t, f.e, http.MethodGet, "/api/events/"+f.eventID+"/participants", "")
	if status != http.StatusOK {
		t.Fatalf("participants returned %d", status)
	}
	participants, _ := resp.Data.([]any)
	if len(participants) != 2 {
		t.Fatalf("want 2 participants, got %d", len(participants))
	}

	// Removing one leaves the record readable from the other.
	status, resp = doJSON(t, f.e, http.MethodDelete, "/api/events/"+f.eventID+"/participants/"+f.persons[1].ID, "")
	if status != http.StatusOK {
		t.Fatalf("removing a participant returned %d: %+v", status, resp.Error)
	}
	remaining, _ := resp.Data.([]any)
	if len(remaining) != 1 {
		t.Fatalf("want 1 participant left, got %d", len(remaining))
	}
	status, _ = doJSON(t, f.e, http.MethodGet, "/api/events/"+f.eventID, "")
	if status != http.StatusOK {
		t.Fatalf("the record itself must survive, GET returned %d", status)
	}

	// Removing the final participant would leave an unreachable record.
	status, resp = doJSON(t, f.e, http.MethodDelete, "/api/events/"+f.eventID+"/participants/"+f.persons[0].ID, "")
	if status != http.StatusBadRequest {
		t.Fatalf("removing the last participant returned %d, want 400 (%+v)", status, resp.Error)
	}
}
