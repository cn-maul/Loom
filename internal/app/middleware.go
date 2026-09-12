package app

import (
	"database/sql"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"relationship/internal/models"
)

// AuthMiddleware guards /api routes with a shared token when one is
// configured. The embedded frontend stays reachable so the SPA can load and
// ask for the token; without a valid token every API call fails closed.
// Accepted credentials: `Authorization: Bearer <token>`, `X-Auth-Token`, or
// `?token=` (the query form exists so a browser link can download an export).
func AuthMiddleware(token string) echo.MiddlewareFunc {
	if token == "" {
		return nil
	}
	denied := func(c echo.Context) error {
		return c.JSON(http.StatusUnauthorized, models.APIResponse{
			OK:    false,
			Error: &models.APIError{Code: "UNAUTHORIZED", Message: "需要访问令牌：请在设置页填入 server.auth_token 配置的令牌"},
		})
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			provided := c.Request().Header.Get("X-Auth-Token")
			if provided == "" {
				if auth := c.Request().Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
					provided = strings.TrimPrefix(auth, "Bearer ")
				}
			}
			if provided == "" {
				provided = c.QueryParam("token")
			}
			if subtleCompare(provided, token) {
				return next(c)
			}
			return denied(c)
		}
	}
}

func subtleCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// sensitivePath reports whether a request touched data worth auditing: every
// mutating call plus the two read paths that export the whole dataset.
func sensitivePath(method, path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return false
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	case http.MethodGet:
		return strings.HasPrefix(path, "/api/export") || strings.HasPrefix(path, "/api/backups/validate")
	}
	return false
}

// AuditMiddleware records sensitive operations into audit_log. A failed audit
// insert logs a warning and never blocks the response it is auditing.
func AuditMiddleware(db *sql.DB) echo.MiddlewareFunc {
	var ready atomic.Bool
	// The table may not exist yet on a fresh database before Migrate runs;
	// NewRouter is always called after wiring, so probe once lazily.
	ready.Store(true)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			err := next(c)
			req := c.Request()
			path := req.URL.Path
			if !sensitivePath(req.Method, path) {
				return err
			}
			if !ready.Load() {
				return err
			}
			_, iErr := db.Exec(
				`INSERT INTO audit_log (id, action, path, status) VALUES (?, ?, ?, ?)`,
				uuid.New().String(), req.Method, path, c.Response().Status,
			)
			if iErr != nil {
				ready.Store(false) // table missing: stop retrying every request
				c.Logger().Warnf("audit insert failed: %v", iErr)
			}
			return err
		}
	}
}
