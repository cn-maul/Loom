package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"relationship/internal/ai"
	"relationship/internal/config"
	"relationship/internal/db"
	"relationship/internal/frontend"
	"relationship/internal/handler"
	"relationship/internal/repository"
	"relationship/internal/service"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	database, err := db.OpenDB(cfg.Database.Path)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	personRepo := repository.NewPersonRepo(database)
	eventRepo := repository.NewEventRepo(database)
	traitRepo := repository.NewTraitRepo(database)
	vecRepo := repository.NewVecRepo(database)
	orgRepo := repository.NewOrganizationRepo(database)

	if err := vecRepo.Init(cfg.LLM.EmbedDim); err != nil {
		// Semantic retrieval is optional; recency and profile context still work.
		log.Printf("WARNING: vector index unavailable: %v", err)
	}

	aiClient := ai.NewClient(&cfg.LLM)

	personService := service.NewPersonService(personRepo, vecRepo, orgRepo)
	eventService := service.NewEventService(eventRepo, vecRepo)
	aiService := service.NewAIService(&cfg.LLM, vecRepo, traitRepo, eventRepo, personRepo, aiClient)
	orgService := service.NewOrganizationService(orgRepo)
	followUpRepo := repository.NewFollowUpRepo(database)
	followUpService := service.NewFollowUpService(followUpRepo, personService)

	personHandler := handler.NewPersonHandler(personService)
	eventHandler := handler.NewEventHandler(eventService, personService, aiService)
	traitHandler := handler.NewTraitHandler(traitRepo)
	aiHandler := handler.NewAIHandler(aiService, personService)
	configHandler := handler.NewConfigHandler(cfg, cfgPath)
	orgHandler := handler.NewOrganizationHandler(orgService)
	followUpHandler := handler.NewFollowUpHandler(followUpService)

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	api := e.Group("/api")
	api.POST("/persons", personHandler.Create)
	api.GET("/persons", personHandler.List)
	api.GET("/persons/:id", personHandler.GetByID)
	api.PUT("/persons/:id", personHandler.Update)
	api.DELETE("/persons/:id", personHandler.Delete)
	api.POST("/organizations", orgHandler.Create)
	api.GET("/organizations", orgHandler.List)
	api.PUT("/organizations/:id", orgHandler.Update)
	api.DELETE("/organizations/:id", orgHandler.Delete)
	api.POST("/events", eventHandler.Create)
	api.GET("/events/:id", eventHandler.GetByID)
	api.DELETE("/events/:id", eventHandler.Delete)
	api.GET("/persons/:id/events", eventHandler.ListByPerson)
	api.GET("/persons/:id/traits", traitHandler.ListByPerson)
	api.PUT("/traits/:id/verify", traitHandler.Verify)
	api.POST("/ai/advice", aiHandler.GetAdvice)
	api.GET("/ai/report/weekly", aiHandler.GetWeeklyReport)
	api.POST("/ai/reindex", aiHandler.Reindex)
	api.GET("/ai/embeddings/status", aiHandler.EmbeddingStatus)
	api.GET("/config", configHandler.Get)
	api.PUT("/config", configHandler.Update)
	api.GET("/follow-ups", followUpHandler.List)
	api.POST("/follow-ups", followUpHandler.Create)
	api.GET("/follow-ups/:id", followUpHandler.Get)
	api.PUT("/follow-ups/:id", followUpHandler.Update)
	api.POST("/follow-ups/:id/complete", followUpHandler.Action("completed"))
	api.POST("/follow-ups/:id/postpone", followUpHandler.Postpone)
	api.POST("/follow-ups/:id/cancel", followUpHandler.Action("cancelled"))
	api.GET("/persons/:id/follow-ups", followUpHandler.ListByPerson)

	if err := serveFrontend(e); err != nil {
		log.Printf("warning: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("server starting on http://%s", addr)
	if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// serveFrontend exposes the embedded build and falls back to index.html so the
// single binary can be started from any directory.
func serveFrontend(e *echo.Echo) error {
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
