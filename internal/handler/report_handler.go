package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"relationship/internal/models"
	"relationship/internal/service"

	"github.com/labstack/echo/v4"
)

// ReportHandler exposes period reports: generating one, re-reading a stored
// snapshot, and pruning the history.
type ReportHandler struct {
	reports *service.ReportService
}

func NewReportHandler(reports *service.ReportService) *ReportHandler {
	return &ReportHandler{reports: reports}
}

// Generate builds a report for the requested window and stores it. A failed
// narrative still answers 201: the report exists and every section is complete,
// and a 5xx would tell the caller they got nothing when they got everything but
// the prose.
func (h *ReportHandler) Generate(c echo.Context) error {
	// A request may legitimately carry no body — the default window is the last
	// seven days — so an empty body is not a binding error.
	req := models.ReportRequest{}
	if err := c.Bind(&req); err != nil && !errors.Is(err, io.EOF) {
		return badRequest(c, "INVALID_INPUT", err.Error())
	}

	// Generation is detached from the request lifecycle: a user who navigates
	// away (or closes the tab) mid-generation must not abort the LLM call and
	// persist a "context canceled" failure. The snapshot lands in the database
	// either way; only the HTTP response is lost, and the client re-reads it
	// from the history on the next visit.
	report, err := h.reports.Generate(context.WithoutCancel(c.Request().Context()), req)
	if err != nil {
		return respondError(c, err, "REPORT_FAILED")
	}
	return c.JSON(http.StatusCreated, models.APIResponse{OK: true, Data: report})
}

// List serves the report history. Without person_id it is the all-people view.
func (h *ReportHandler) List(c echo.Context) error {
	snapshots, err := h.reports.List(c.QueryParam("person_id"), queryLimit(c, "limit", 0, MaxPageLimit), queryInt(c, "offset", 0))
	if err != nil {
		return respondError(c, err, "LIST_FAILED")
	}
	if snapshots == nil {
		snapshots = []*models.ReportSnapshot{}
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: snapshots})
}

func (h *ReportHandler) Get(c echo.Context) error {
	report, err := h.reports.Get(c.Param("id"))
	if err != nil {
		return respondError(c, err, "READ_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: report})
}

func (h *ReportHandler) Delete(c echo.Context) error {
	if err := h.reports.Delete(c.Param("id")); err != nil {
		return respondError(c, err, "DELETE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true})
}
