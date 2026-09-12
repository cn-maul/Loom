package handler

import (
	"net/http"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/labstack/echo/v4"
)

// DashboardHandler serves the landing-page aggregate: counts plus the latest
// person, profile change and record. Everything is read-only and cheap.
type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

func (h *DashboardHandler) Stats(c echo.Context) error {
	stats, err := h.svc.Stats()
	if err != nil {
		return respondError(c, err, "STATS_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: stats})
}