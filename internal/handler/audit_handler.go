package handler

import (
	"database/sql"
	"net/http"

	"relationship/internal/models"

	"github.com/labstack/echo/v4"
)

type AuditEntry struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
}

type AuditHandler struct {
	db *sql.DB
}

func NewAuditHandler(db *sql.DB) *AuditHandler {
	return &AuditHandler{db: db}
}

// List returns recent audit entries, newest first.
func (h *AuditHandler) List(c echo.Context) error {
	limit := queryLimit(c, "limit", 100, 500)
	if limit <= 0 {
		limit = 100
	}
	rows, err := h.db.Query(
		`SELECT id, action, path, status, created_at FROM audit_log ORDER BY created_at DESC, id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return respondError(c, err, "AUDIT_LIST_FAILED")
	}
	defer rows.Close()
	entries := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Action, &e.Path, &e.Status, &e.CreatedAt); err != nil {
			return respondError(c, err, "AUDIT_LIST_FAILED")
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return respondError(c, err, "AUDIT_LIST_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{OK: true, Data: map[string]interface{}{"entries": entries}})
}
