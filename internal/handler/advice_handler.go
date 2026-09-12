package handler

import (
	"net/http"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/labstack/echo/v4"
)

// AdviceHandler exposes the stored advice history: generating, re-reading,
// adopting a strategy and deleting an answer.
type AdviceHandler struct {
	advice *service.AdviceService
}

func NewAdviceHandler(advice *service.AdviceService) *AdviceHandler {
	return &AdviceHandler{advice: advice}
}

// Generate answers a question and returns the stored session, so the caller
// immediately has the id it needs to adopt a strategy or come back to it later.
func (h *AdviceHandler) Generate(c echo.Context) error {
	var req models.AdviceRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	session, err := h.advice.Generate(c.Request().Context(), req)
	if err != nil {
		return respondError(c, err, "ADVICE_FAILED")
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: session})
}

// List serves the advice history. Without person_id it is the all-people view.
func (h *AdviceHandler) List(c echo.Context) error {
	sessions, err := h.advice.List(c.QueryParam("person_id"), queryLimit(c, "limit", 0, MaxPageLimit), queryInt(c, "offset", 0))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if sessions == nil {
		sessions = []*models.AdviceSession{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: sessions})
}

func (h *AdviceHandler) Get(c echo.Context) error {
	session, err := h.advice.Get(c.Param("id"))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: session})
}

func (h *AdviceHandler) Delete(c echo.Context) error {
	if err := h.advice.Delete(c.Param("id")); err != nil {
		return respondError(c, err, "DELETE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}

// Adopt turns a chosen strategy into a follow-up, which is where an answer stops
// being advice and starts being something the user has to do.
func (h *AdviceHandler) Adopt(c echo.Context) error {
	var req models.AdoptRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	session, err := h.advice.Adopt(c.Param("id"), req)
	if err != nil {
		return respondError(c, err, "ADOPT_FAILED")
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: session})
}
