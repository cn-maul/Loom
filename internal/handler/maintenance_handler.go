package handler

import (
	"net/http"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/labstack/echo/v4"
)

// MaintenanceHandler exposes the data-hygiene pass. It runs automatically at
// startup; the endpoint exists so an operator can run it on demand and see
// the numbers without hunting through logs.
type MaintenanceHandler struct {
	svc *service.MaintenanceService
}

func NewMaintenanceHandler(svc *service.MaintenanceService) *MaintenanceHandler {
	return &MaintenanceHandler{svc: svc}
}

// Cleanup runs one maintenance pass and returns what it removed.
func (h *MaintenanceHandler) Cleanup(c echo.Context) error {
	report, err := h.svc.Cleanup()
	if err != nil {
		return respondError(c, err, "CLEANUP_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: map[string]interface{}{"report": report}})
}
