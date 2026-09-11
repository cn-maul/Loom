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

// Get serves the whole /relationships canvas in one payload: person nodes,
// active organisation nodes, structured edges and the derived co-attendance
// pairs. Filters live on the client because the graph is small enough to ship
// whole and the filter set keeps changing with the UI.
func (h *GraphHandler) Get(c echo.Context) error {
	data, err := h.service.Build()
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: data})
}
