package handler

import (
	"net/http"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/labstack/echo/v4"
)

type GraphHandler struct {
	service *service.GraphService
}

func NewGraphHandler(service *service.GraphService) *GraphHandler {
	return &GraphHandler{service: service}
}

// Get serves the /relationships canvas in one payload: person nodes, active
// organisation nodes, structured edges and the derived co-attendance pairs.
// Filters live on the client; ?limit= caps the person nodes (default 1000,
// hard cap 5000) and the response reports truncation instead of hiding it.
func (h *GraphHandler) Get(c echo.Context) error {
	data, err := h.service.Build(queryLimit(c, "limit", service.DefaultMaxNodes, service.HardMaxNodes))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: data})
}
