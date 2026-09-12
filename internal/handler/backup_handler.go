// backup_handler.go exposes the data-lifecycle endpoints: snapshots, export,
// and two-phase restore. Restore is intentionally staged rather than applied
// live — the handler never swaps the database under a running process.
package handler

import (
	"fmt"
	"net/http"
	"time"

	"relationship/internal/backup"
	"relationship/internal/models"

	"github.com/labstack/echo/v4"
)

type BackupHandler struct {
	service *backup.Service
}

func NewBackupHandler(service *backup.Service) *BackupHandler {
	return &BackupHandler{service: service}
}

// Create takes a snapshot right now.
func (h *BackupHandler) Create(c echo.Context) error {
	info, err := h.service.Create()
	if err != nil {
		return respondError(c, err, "BACKUP_FAILED")
	}
	return c.JSON(http.StatusCreated, models.APIResponse{
		OK:   true,
		Data: map[string]interface{}{"backup": info},
	})
}

// List returns the snapshots in the backup directory, newest first.
func (h *BackupHandler) List(c echo.Context) error {
	list, err := h.service.List()
	if err != nil {
		return respondError(c, err, "BACKUP_LIST_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{
		OK:   true,
		Data: map[string]interface{}{"backups": list},
	})
}

// Status reports scheduler state: last run, last error, next run. This is the
// "自动备份失败有明确可见提示" surface — a silent backup gap is not acceptable.
func (h *BackupHandler) Status(c echo.Context) error {
	return c.JSON(http.StatusOK, models.APIResponse{
		OK:   true,
		Data: map[string]interface{}{"status": h.service.Status()},
	})
}

type backupFileRequest struct {
	File string `json:"file"`
}

// Validate runs the pre-restore checks against one file and reports the
// result without staging anything.
func (h *BackupHandler) Validate(c echo.Context) error {
	var req backupFileRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	if req.File == "" {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: "file is required"},
		})
	}
	rep, err := h.service.ValidateFile(h.service.ResolvePath(req.File))
	if err != nil {
		return respondError(c, err, "BACKUP_VALIDATE_FAILED")
	}
	return c.JSON(http.StatusOK, models.APIResponse{
		OK:   true,
		Data: map[string]interface{}{"report": rep},
	})
}

// Restore validates and stages a snapshot. The response says so explicitly:
// the swap happens on the next startup, never mid-request.
func (h *BackupHandler) Restore(c echo.Context) error {
	var req backupFileRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	if req.File == "" {
		return c.JSON(http.StatusBadRequest, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "INVALID_INPUT", Message: "file is required"},
		})
	}
	rep, err := h.service.StageRestore(req.File)
	if err != nil {
		if rep != nil {
			// Validation failed: return the full report so the user sees why.
			return c.JSON(http.StatusUnprocessableEntity, models.APIResponse{
				OK:    false,
				Data:  map[string]interface{}{"report": rep},
				Error: &models.APIError{Code: "BACKUP_REJECTED", Message: err.Error()},
			})
		}
		return respondError(c, err, "BACKUP_RESTORE_FAILED")
	}
	return c.JSON(http.StatusAccepted, models.APIResponse{
		OK:   true,
		Data: map[string]interface{}{
			"report":  rep,
			"message": "恢复已暂存，重启 Loom 后生效。重启前当前数据保持不变。",
		},
	})
}

// Export streams the whole dataset as JSON. mode=redacted nulls sensitive
// text and names before serialization.
func (h *BackupHandler) Export(c echo.Context) error {
	redacted := c.QueryParam("mode") == "redacted"
	data, err := h.service.ExportJSON(redacted)
	if err != nil {
		return respondError(c, err, "EXPORT_FAILED")
	}
	mode := "full"
	if redacted {
		mode = "redacted"
	}
	name := fmt.Sprintf("loom-export-%s-%s.json", mode, time.Now().UTC().Format("20060102-150405"))
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	return c.Blob(http.StatusOK, "application/json; charset=utf-8", data)
}
