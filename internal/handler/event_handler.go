package handler

import (
	"encoding/json"
	"net/http"
	"relationship/internal/models"
	"relationship/internal/service"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type EventHandler struct {
	eventService *service.EventService
	aiService    *service.AIService
}

func NewEventHandler(eventService *service.EventService, aiService *service.AIService) *EventHandler {
	return &EventHandler{eventService: eventService, aiService: aiService}
}

func (h *EventHandler) Create(c echo.Context) error {
	var event models.Event
	if err := c.Bind(&event); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()},
		})
	}

	event.ID = uuid.New().String()

	// AI提取
	extraction, err := h.aiService.ExtractEvent(c.Request().Context(), event.RawText)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "AI_EXTRACTION_FAILED", Message: err.Error()},
		})
	}

	event.Summary = extraction.Summary
	event.MyFeeling = extraction.MyFeeling
	event.TheirReaction = extraction.TheirReaction

	// Convert promises to JSON string
	promisesJSON, _ := json.Marshal(extraction.Promises)
	event.Promises = string(promisesJSON)

	if err := h.eventService.Create(&event); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "CREATE_FAILED", Message: err.Error()},
		})
	}

	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: event})
}

func (h *EventHandler) GetByID(c echo.Context) error {
	id := c.Param("id")
	event, err := h.eventService.GetByID(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "NOT_FOUND", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: event})
}

func (h *EventHandler) ListByPerson(c echo.Context) error {
	personID := c.Param("id")
	limitStr := c.QueryParam("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	events, err := h.eventService.ListByPerson(personID, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "LIST_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: events})
}

func (h *EventHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if err := h.eventService.Delete(id); err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{
			OK:   false,
			Error: &models.APIError{Code: "DELETE_FAILED", Message: err.Error()},
		})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}