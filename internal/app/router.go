package app

import (
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"relationship/internal/frontend"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// NewRouter builds the Echo instance: middleware, the /api routes and the
// embedded frontend with an index.html fallback for SPA paths.
func (a *App) NewRouter() (*echo.Echo, error) {
	e := echo.New()
	e.Use(
		middleware.Logger(),
		middleware.Recover(),
		middleware.CORS(),
		ObserveMiddleware(),
	)
	// Audit before auth: a rejected request (401) still touched a sensitive
	// path and is worth a row.
	if m := AuditMiddleware(a.DB); m != nil {
		e.Use(m)
	}

	a.registerAPI(e)

	if err := a.serveFrontend(e); err != nil {
		return nil, err
	}
	return e, nil
}

// registerAPI maps every HTTP route to its handler. Route grouping follows the
// domain order used across the codebase: people and organisations first, then
// records, structured relations, profile utilities, advice, follow-ups and
// reports.
func (a *App) registerAPI(e *echo.Echo) {
	// Auth is group-scoped: the token guards /api only, so the embedded SPA
	// still loads and can ask the user for the token.
	var groupMw []echo.MiddlewareFunc
	if m := AuthMiddleware(a.Config.Server.AuthToken); m != nil {
		groupMw = append(groupMw, m)
	}
	api := e.Group("/api", groupMw...)

	// Security-sensitive operations are listed for visibility.
	api.GET("/audit", a.auditHandler.List)

	// People and organisations.
	api.POST("/persons", a.personHandler.Create)
	api.GET("/persons", a.personHandler.List)
	api.GET("/persons/:id", a.personHandler.GetByID)
	api.PUT("/persons/:id", a.personHandler.Update)
	api.DELETE("/persons/:id", a.personHandler.Delete)
	api.POST("/organizations", a.orgHandler.Create)
	api.GET("/organizations", a.orgHandler.List)
	api.PUT("/organizations/:id", a.orgHandler.Update)
	api.POST("/organizations/:id/archive", a.orgHandler.Archive)
	api.POST("/organizations/:id/restore", a.orgHandler.Restore)
	api.DELETE("/organizations/:id", a.orgHandler.Delete)

	// Records, with the attendance list that makes one record readable from
	// every participant.
	api.POST("/events", a.eventHandler.Create)
	api.GET("/events", a.eventHandler.List)
	api.GET("/events/:id", a.eventHandler.GetByID)
	api.PUT("/events/:id", a.eventHandler.Update)
	api.DELETE("/events/:id", a.eventHandler.Delete)
	api.GET("/events/:id/participants", a.eventHandler.ListParticipants)
	api.PUT("/events/:id/participants", a.eventHandler.SetParticipants)
	api.DELETE("/events/:id/participants/:personID", a.eventHandler.RemoveParticipant)
	api.POST("/events/:id/extract", a.eventHandler.RetryExtract)
	api.GET("/persons/:id/events", a.eventHandler.ListByPerson)

	// Structured relationships and organisation postings.
	api.POST("/relationships", a.relationshipHandler.Create)
	api.GET("/relationships/types", a.relationshipHandler.ListTypes)
	api.PUT("/relationships/:id", a.relationshipHandler.Update)
	api.DELETE("/relationships/:id", a.relationshipHandler.Delete)
	api.GET("/persons/:id/relationships", a.relationshipHandler.ListByPerson)
	api.GET("/graph", a.graphHandler.Get)
	api.POST("/persons/:id/positions", a.positionHandler.Create)
	api.GET("/persons/:id/positions", a.positionHandler.ListByPerson)
	api.GET("/organizations/:id/members", a.positionHandler.ListByOrg)
	api.PUT("/positions/:id", a.positionHandler.Update)
	api.DELETE("/positions/:id", a.positionHandler.Delete)

	// Person profile and AI utilities.
	api.GET("/persons/:id/traits", a.traitHandler.ListByPerson)
	api.PUT("/traits/:id/verify", a.traitHandler.Verify)
	api.POST("/ai/reindex", a.aiHandler.Reindex)
	api.GET("/ai/embeddings/status", a.aiHandler.EmbeddingStatus)
	api.GET("/ai/models", a.aiHandler.ListModels)
	api.POST("/ai/infer-hierarchy", a.aiHandler.InferHierarchy)
	api.GET("/config", a.configHandler.Get)
	api.PUT("/config", a.configHandler.Update)

	// Backup, export and two-phase restore.
	api.POST("/backups", a.backupHandler.Create)
	api.GET("/backups", a.backupHandler.List)
	api.GET("/backups/status", a.backupHandler.Status)
	api.POST("/backups/validate", a.backupHandler.Validate)
	api.POST("/backups/restore", a.backupHandler.Restore)
	api.GET("/export", a.backupHandler.Export)

	// Advice is generated and kept: the answer, the context it used, the model
	// that wrote it, and the follow-ups it turned into.
	api.POST("/advice", a.adviceHandler.Generate)
	api.GET("/advice", a.adviceHandler.List)
	api.GET("/advice/:id", a.adviceHandler.Get)
	api.DELETE("/advice/:id", a.adviceHandler.Delete)
	api.POST("/advice/:id/adopt", a.adviceHandler.Adopt)

	// Follow-ups.
	api.GET("/follow-ups", a.followUpHandler.List)
	api.POST("/follow-ups", a.followUpHandler.Create)
	api.GET("/follow-ups/:id", a.followUpHandler.Get)
	api.PUT("/follow-ups/:id", a.followUpHandler.Update)
	api.GET("/follow-ups/:id/postponements", a.followUpHandler.ListPostponements)
	api.POST("/follow-ups/:id/complete", a.followUpHandler.Complete)
	api.POST("/follow-ups/:id/postpone", a.followUpHandler.Postpone)
	api.POST("/follow-ups/:id/wait", a.followUpHandler.Action("waiting"))
	api.POST("/follow-ups/:id/cancel", a.followUpHandler.Action("cancelled"))
	api.GET("/persons/:id/follow-ups", a.followUpHandler.ListByPerson)

	// Reports: structured aggregation first, narration second, both persisted.
	api.POST("/reports", a.reportHandler.Generate)
	api.GET("/reports", a.reportHandler.List)
	api.GET("/reports/:id", a.reportHandler.Get)
	api.DELETE("/reports/:id", a.reportHandler.Delete)
}

// serveFrontend exposes the embedded build and falls back to index.html so the
// single binary can be started from any directory.
func (a *App) serveFrontend(e *echo.Echo) error {
	frontendFS, err := frontend.GetFS()
	if err != nil {
		return fmt.Errorf("load frontend: %w", err)
	}

	index, err := fs.ReadFile(frontendFS, "index.html")
	if err != nil {
		return fmt.Errorf("frontend build is missing (index.html): %w", err)
	}

	e.GET("/*", func(c echo.Context) error {
		name := strings.TrimPrefix(c.Request().URL.Path, "/")
		if name == "" {
			return c.HTML(http.StatusOK, string(index))
		}
		data, err := fs.ReadFile(frontendFS, name)
		if err != nil {
			return c.HTML(http.StatusOK, string(index))
		}
		return c.Blob(http.StatusOK, contentType(name, data), data)
	})
	return nil
}

func contentType(name string, data []byte) string {
	switch strings.ToLower(pathExt(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	case ".woff2":
		return "font/woff2"
	default:
		return http.DetectContentType(data)
	}
}

func pathExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}
