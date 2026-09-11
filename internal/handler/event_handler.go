package handler

import (
	"net/http"
	"relationship/internal/models"
	"relationship/internal/service"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type EventHandler struct {
	eventService  *service.EventService
	personService *service.PersonService
	aiService     *service.AIService
}

func NewEventHandler(
	eventService *service.EventService,
	personService *service.PersonService,
	aiService *service.AIService,
) *EventHandler {
	return &EventHandler{
		eventService:  eventService,
		personService: personService,
		aiService:     aiService,
	}
}

// Create records a raw note and runs the AI pipeline over it. The pipeline reports
// partial success rather than failing the record, so the user's text is never lost.
func (h *EventHandler) Create(c echo.Context) error {
	var event models.Event
	if err := c.Bind(&event); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}
	event.ID = uuid.New().String()
	if err := event.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}
	// Fail before the LLM round-trips when the person does not exist.
	if _, err := h.personService.GetByID(event.PersonID); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "PERSON_NOT_FOUND", Message: "unknown person_id"},
		})
	}

	report, err := h.aiService.IngestEvent(c.Request().Context(), &event)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "CREATE_FAILED", Message: err.Error()},
		})
	}

	return c.JSON(http.StatusCreated, models.APIResponse{
		OK:   true,
		Data: map[string]interface{}{"event": event, "report": report},
	})
}

func (h *EventHandler) GetByID(c echo.Context) error {
	event, err := h.eventService.GetByID(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "NOT_FOUND", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: event})
}

func (h *EventHandler) ListByPerson(c echo.Context) error {
	limit := 50
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	events, err := h.eventService.ListByPerson(c.Param("id"), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "LIST_FAILED", Message: err.Error()},
		})
	}
	if events == nil {
		events = []*models.Event{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: events})
}

func (h *EventHandler) Delete(c echo.Context) error {
	if err := h.eventService.Delete(c.Param("id")); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "DELETE_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}
