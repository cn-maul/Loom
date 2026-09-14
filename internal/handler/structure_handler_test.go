package handler

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"relationship/internal/config"
	"relationship/internal/db"
	"relationship/internal/models"
	"relationship/internal/repository"
	"relationship/internal/service"
)

type structureFixture struct {
	e       *echo.Echo
	persons []*models.Person
	eventID string
}

// newStructureAPI wires record participants against a throwaway database, so
// these tests never touch data/ or any developer database.
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
	eventService := service.NewEventService(
		repository.NewEventRepo(database),
		repository.NewVecRepo(database),
		personService,
		repository.NewTraitRepo(database),
	)

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

	// This fixture never hits the create/extract paths, so a sync-mode ingest
	// with no AI service is enough to satisfy the constructor.
	eventHandler := NewEventHandler(eventService, nil, service.NewIngestService(nil, &config.LLMConfig{AsyncExtract: false}))

	e := echo.New()
	api := e.Group("/api")
	api.GET("/events/:id", eventHandler.GetByID)
	api.GET("/events/:id/participants", eventHandler.ListParticipants)
	api.PUT("/events/:id/participants", eventHandler.SetParticipants)
	api.DELETE("/events/:id/participants/:personID", eventHandler.RemoveParticipant)

	return &structureFixture{e: e, persons: persons, eventID: event.ID}
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

// Every failure mode of the participant API reports its own status and code,
// because a caller that cannot tell "no such record" from "you sent nonsense"
// will retry the wrong one.
func TestParticipantAPIStatusClassification(t *testing.T) {
	f := newStructureAPI(t)
	ghost := uuid.New().String()

	cases := []struct {
		name   string
		method string
		target string
		body   string
		want   int
		code   string
	}{
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
}
