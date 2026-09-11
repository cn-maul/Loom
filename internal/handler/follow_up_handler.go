package handler

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"relationship/internal/models"
	"relationship/internal/service"
)

type FollowUpHandler struct{ service *service.FollowUpService }

func NewFollowUpHandler(s *service.FollowUpService) *FollowUpHandler {
	return &FollowUpHandler{service: s}
}
func (h *FollowUpHandler) List(c echo.Context) error {
	items, err := h.service.List(c.QueryParam("person_id"), c.QueryParam("status"), c.QueryParam("from"), c.QueryParam("to"))
	if err != nil {
		return c.JSON(500, models.APIResponse{OK: false, Error: &models.APIError{Code: "LIST_FAILED", Message: err.Error()}})
	}
	if items == nil {
		items = []*models.FollowUp{}
	}
	return c.JSON(200, models.APIResponse{OK: true, Data: items})
}
func (h *FollowUpHandler) ListByPerson(c echo.Context) error {
	c.SetPath("/api/follow-ups")
	return h.listPerson(c)
}
func (h *FollowUpHandler) listPerson(c echo.Context) error {
	items, err := h.service.List(c.Param("id"), c.QueryParam("status"), c.QueryParam("from"), c.QueryParam("to"))
	if err != nil {
		return c.JSON(500, models.APIResponse{OK: false, Error: &models.APIError{Code: "LIST_FAILED", Message: err.Error()}})
	}
	if items == nil {
		items = []*models.FollowUp{}
	}
	return c.JSON(200, models.APIResponse{OK: true, Data: items})
}
func (h *FollowUpHandler) Get(c echo.Context) error {
	f, err := h.service.Get(c.Param("id"))
	if err != nil {
		return c.JSON(404, models.APIResponse{OK: false, Error: &models.APIError{Code: "NOT_FOUND", Message: err.Error()}})
	}
	return c.JSON(200, models.APIResponse{OK: true, Data: f})
}
func (h *FollowUpHandler) Create(c echo.Context) error {
	var f models.FollowUp
	if err := c.Bind(&f); err != nil {
		return c.JSON(400, models.APIResponse{OK: false, Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()}})
	}
	if err := h.service.Create(&f); err != nil {
		return c.JSON(400, models.APIResponse{OK: false, Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()}})
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: f})
}
func (h *FollowUpHandler) Update(c echo.Context) error {
	var f models.FollowUp
	if err := c.Bind(&f); err != nil {
		return c.JSON(400, models.APIResponse{OK: false, Error: &models.APIError{Code: "INVALID_INPUT", Message: err.Error()}})
	}
	f.ID = c.Param("id")
	if err := h.service.Update(&f); err != nil {
		return c.JSON(400, models.APIResponse{OK: false, Error: &models.APIError{Code: "UPDATE_FAILED", Message: err.Error()}})
	}
	return c.JSON(200, models.APIResponse{OK: true, Data: f})
}
func (h *FollowUpHandler) Postpone(c echo.Context) error {
	var req struct {
		DueDate string `json:"due_date"`
	}
	if err := c.Bind(&req); err != nil || req.DueDate == "" {
		return c.JSON(http.StatusBadRequest, models.APIResponse{OK: false, Error: &models.APIError{Code: "INVALID_INPUT", Message: "due_date is required"}})
	}
	if err := h.service.Postpone(c.Param("id"), req.DueDate); err != nil {
		return c.JSON(http.StatusBadRequest, models.APIResponse{OK: false, Error: &models.APIError{Code: "POSTPONE_FAILED", Message: err.Error()}})
	}
	item, err := h.service.Get(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, models.APIResponse{OK: false, Error: &models.APIError{Code: "READ_FAILED", Message: err.Error()}})
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: item})
}

func (h *FollowUpHandler) Action(status string) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := h.service.Action(c.Param("id"), status); err != nil {
			return c.JSON(400, models.APIResponse{OK: false, Error: &models.APIError{Code: "ACTION_FAILED", Message: err.Error()}})
		}
		return c.JSON(200, models.APIResponse{OK: true})
	}
}
