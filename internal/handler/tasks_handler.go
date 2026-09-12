package handler

import (
	"net/http"

	"relationship/internal/models"
	"relationship/internal/service"
	"relationship/internal/tasks"

	"github.com/labstack/echo/v4"
)

// TasksHandler exposes the extraction queue for observability: which records
// are waiting, running, done or failed, and why. It answers the question the
// pending badge on a record raises — "is it actually being worked on?"
type TasksHandler struct {
	ingest *service.IngestService
}

func NewTasksHandler(ingest *service.IngestService) *TasksHandler {
	return &TasksHandler{ingest: ingest}
}

// List serves the recent task history, newest first, plus queue-level stats.
// limit caps the history window. The response shape is `{tasks, stats}` —
// stats carries lifetime counters and run latency so a failing pipeline is a
// number on a page, not a rumour in the logs.
func (h *TasksHandler) List(c echo.Context) error {
	limit := queryLimit(c, "limit", 50, MaxPageLimit)
	recent := h.ingest.RecentTasks(limit)
	if recent == nil {
		recent = make([]*tasks.Task, 0)
	}
	return c.JSON(http.StatusOK, models.APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"tasks": recent,
			"stats": h.ingest.Stats(),
		},
	})
}
