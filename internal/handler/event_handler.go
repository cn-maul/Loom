package handler

import (
	"net/http"
	"strconv"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type EventHandler struct {
	eventService *service.EventService
	aiService    *service.AIService
	ingest       *service.IngestService
}

func NewEventHandler(eventService *service.EventService, aiService *service.AIService, ingest *service.IngestService) *EventHandler {
	return &EventHandler{eventService: eventService, aiService: aiService, ingest: ingest}
}

// Create records a raw note and runs the AI pipeline over it. The pipeline reports
// partial success rather than failing the record, so the user's text is never lost.
// The record may name several participants; it is stored once and appears on every
// attendee's timeline.
//
// In async mode (default) the record is stored and returned immediately with
// extraction_status='pending' and report.async=true; extraction, indexing and the
// profile refresh run in the background queue. Sync mode blocks until the
// pipeline finishes, as before.
func (h *EventHandler) Create(c echo.Context) error {
	var event models.Event
	if err := bindJSON(c, &event); err != nil {
		return err
	}
	event.ID = uuid.New().String()
	if err := event.Validate(); err != nil {
		return respondError(c, err, "INVALID_INPUT")
	}
	// Fail before any LLM work when a participant does not exist.
	if err := h.eventService.EnsurePeopleExist(event.Participants); err != nil {
		return respondError(c, err, "PERSON_NOT_FOUND")
	}

	report, err := h.ingest.Create(&event)
	if err != nil {
		return respondError(c, err, "CREATE_FAILED")
	}

	return c.JSON(http.StatusCreated, models.APIResponse{
		OK:   true,
		Data: map[string]interface{}{"event": event, "report": report},
	})
}

// List serves the record list with time range, keyword, extraction state and
// page window. The match count travels in X-Total-Count so a paginated caller
// can size the rest of the list without a second query.
func (h *EventHandler) List(c echo.Context) error {
	events, total, err := h.eventService.List(models.EventFilter{
		PersonID: c.QueryParam("person_id"),
		OrgID:    c.QueryParam("org_id"),
		From:     c.QueryParam("from"),
		To:       c.QueryParam("to"),
		Query:    c.QueryParam("q"),
		Status:   c.QueryParam("status"),
		Limit:    queryLimit(c, "limit", 0, MaxPageLimit),
		Offset:   queryInt(c, "offset", 0),
	})
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if events == nil {
		events = []*models.Event{}
	}
	c.Response().Header().Set("X-Total-Count", strconv.Itoa(total))
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: events})
}

func (h *EventHandler) GetByID(c echo.Context) error {
	event, err := h.eventService.GetByID(c.Param("id"))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: event})
}

// Update edits the record's own fields. Attendance is managed through the
// participant endpoints so an edit cannot silently drop co-attendees.
func (h *EventHandler) Update(c echo.Context) error {
	var event models.Event
	if err := bindJSON(c, &event); err != nil {
		return err
	}
	event.ID = c.Param("id")
	if err := h.eventService.Update(&event); err != nil {
		return respondError(c, err, "UPDATE_FAILED")
	}
	updated, err := h.eventService.GetByID(event.ID)
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: updated})
}

func (h *EventHandler) ListByPerson(c echo.Context) error {
	events, err := h.eventService.ListByPerson(c.Param("id"), queryLimit(c, "limit", 50, MaxPageLimit))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if events == nil {
		events = []*models.Event{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: events})
}

func (h *EventHandler) Delete(c echo.Context) error {
	if err := h.eventService.Delete(c.Param("id")); err != nil {
		return respondError(c, err, "DELETE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}

func (h *EventHandler) ListParticipants(c echo.Context) error {
	participants, err := h.eventService.ListParticipants(c.Param("id"))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: participants})
}

// SetParticipants replaces the whole attendance list; the record itself is not
// duplicated, so every participant sees the same text.
func (h *EventHandler) SetParticipants(c echo.Context) error {
	var body struct {
		Participants []models.EventParticipant `json:"participants"`
	}
	if err := bindJSON(c, &body); err != nil {
		return err
	}
	participants, err := h.eventService.SetParticipants(c.Param("id"), body.Participants)
	if err != nil {
		return respondError(c, err, "UPDATE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: participants})
}

func (h *EventHandler) RemoveParticipant(c echo.Context) error {
	participants, err := h.eventService.RemoveParticipant(c.Param("id"), c.Param("personID"))
	if err != nil {
		return respondError(c, err, "UPDATE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: participants})
}

// RetryExtract re-runs the extraction for one record. A record whose stored
// result was written by hand is refused unless the caller sets force, because
// silently replacing a correction is the failure this endpoint exists to avoid.
// The record itself is returned either way, versioned by its new status.
func (h *EventHandler) RetryExtract(c echo.Context) error {
	var req struct {
		Force bool `json:"force"`
	}
	if c.Request().ContentLength > 0 {
		if err := bindJSON(c, &req); err != nil {
			return err
		}
	}
	event, report, err := h.aiService.RetryExtraction(c.Request().Context(), c.Param("id"), req.Force)
	if err != nil {
		return respondError(c, err, "EXTRACT_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{
		OK:   true,
		Data: map[string]interface{}{"event": event, "report": report},
	})
}
