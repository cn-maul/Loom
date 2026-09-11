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
	orgService := service.NewOrganizationService(orgRepo)
	eventService := service.NewEventService(eventRepo, vecRepo, personService, traitRepo)
	aiService := service.NewAIService(&cfg.LLM, vecRepo, traitRepo, eventRepo, personRepo, aiClient)
	followUpRepo := repository.NewFollowUpRepo(database)
	followUpService := service.NewFollowUpService(followUpRepo, personService, eventRepo)
	relationshipService := service.NewRelationshipService(repository.NewRelationshipRepo(database), personService, eventService)
	positionService := service.NewPositionService(repository.NewPositionRepo(database), personService, orgService)
	graphService := service.NewGraphService(personRepo, repository.NewGraphRepo(database))
	adviceService := service.NewAdviceService(repository.NewAdviceRepo(database), eventRepo, followUpRepo, personService, aiService)
	reportService := service.NewReportService(
		repository.NewReportRepo(database), eventRepo, followUpRepo,
		repository.NewRelationshipRepo(database), repository.NewPositionRepo(database),
		personService, aiService,
	)

	personHandler := handler.NewPersonHandler(personService)
	eventHandler := handler.NewEventHandler(eventService, aiService)
	traitHandler := handler.NewTraitHandler(traitRepo)
	aiHandler := handler.NewAIHandler(aiService)
	configHandler := handler.NewConfigHandler(cfg, cfgPath)
	orgHandler := handler.NewOrganizationHandler(orgService)
	followUpHandler := handler.NewFollowUpHandler(followUpService)
	relationshipHandler := handler.NewRelationshipHandler(relationshipService)
	positionHandler := handler.NewPositionHandler(positionService)
	graphHandler := handler.NewGraphHandler(graphService)
	adviceHandler := handler.NewAdviceHandler(adviceService)
	reportHandler := handler.NewReportHandler(reportService)

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	api := e.Group("/api")

	// People and organisations.
	api.POST("/persons", personHandler.Create)
	api.GET("/persons", personHandler.List)
	api.GET("/persons/:id", personHandler.GetByID)
	api.PUT("/persons/:id", personHandler.Update)
	api.DELETE("/persons/:id", personHandler.Delete)
	api.POST("/organizations", orgHandler.Create)
	api.GET("/organizations", orgHandler.List)
	api.GET("/organizations/:id", orgHandler.GetByID)
	api.PUT("/organizations/:id", orgHandler.Update)
	api.POST("/organizations/:id/archive", orgHandler.Archive)
	api.POST("/organizations/:id/restore", orgHandler.Restore)
	api.DELETE("/organizations/:id", orgHandler.Delete)

	// Records, with the attendance list that makes one record readable from every
	// participant.
	api.POST("/events", eventHandler.Create)
	api.GET("/events", eventHandler.List)
	api.GET("/events/:id", eventHandler.GetByID)
	api.PUT("/events/:id", eventHandler.Update)
	api.DELETE("/events/:id", eventHandler.Delete)
	api.GET("/events/:id/participants", eventHandler.ListParticipants)
	api.PUT("/events/:id/participants", eventHandler.SetParticipants)
	api.DELETE("/events/:id/participants/:personID", eventHandler.RemoveParticipant)
	api.POST("/events/:id/extract", eventHandler.RetryExtract)
	api.GET("/persons/:id/events", eventHandler.ListByPerson)

	// Structured relationships and organisation postings.
	api.POST("/relationships", relationshipHandler.Create)
	api.GET("/relationships", relationshipHandler.List)
	api.GET("/relationships/types", relationshipHandler.ListTypes)
	api.GET("/relationships/co-attendance", relationshipHandler.CoAttendance)
	api.GET("/relationships/:id", relationshipHandler.Get)
	api.PUT("/relationships/:id", relationshipHandler.Update)
	api.DELETE("/relationships/:id", relationshipHandler.Delete)
	api.GET("/persons/:id/relationships", relationshipHandler.ListByPerson)
	api.GET("/graph", graphHandler.Get)
	api.POST("/persons/:id/positions", positionHandler.Create)
	api.GET("/persons/:id/positions", positionHandler.ListByPerson)
	api.GET("/organizations/:id/members", positionHandler.ListByOrg)
	api.GET("/positions/:id", positionHandler.Get)
	api.PUT("/positions/:id", positionHandler.Update)
	api.DELETE("/positions/:id", positionHandler.Delete)

	// Person profile and AI utilities.
	api.GET("/persons/:id/traits", traitHandler.ListByPerson)
	api.PUT("/traits/:id/verify", traitHandler.Verify)
	api.POST("/ai/reindex", aiHandler.Reindex)
	api.GET("/ai/embeddings/status", aiHandler.EmbeddingStatus)
	api.GET("/config", configHandler.Get)
	api.PUT("/config", configHandler.Update)

	// Advice is generated and kept: the answer, the context it used, the model
	// that wrote it, and the follow-ups it turned into.
	api.POST("/advice", adviceHandler.Generate)
	api.GET("/advice", adviceHandler.List)
	api.GET("/advice/:id", adviceHandler.Get)
	api.DELETE("/advice/:id", adviceHandler.Delete)
	api.POST("/advice/:id/adopt", adviceHandler.Adopt)

	// Follow-ups.
	api.GET("/follow-ups", followUpHandler.List)
	api.POST("/follow-ups", followUpHandler.Create)
	api.GET("/follow-ups/:id", followUpHandler.Get)
	api.PUT("/follow-ups/:id", followUpHandler.Update)
	api.GET("/follow-ups/:id/postponements", followUpHandler.ListPostponements)
	api.POST("/follow-ups/:id/complete", followUpHandler.Complete)
	api.POST("/follow-ups/:id/postpone", followUpHandler.Postpone)
	api.POST("/follow-ups/:id/wait", followUpHandler.Action("waiting"))
	api.POST("/follow-ups/:id/cancel", followUpHandler.Action("cancelled"))
	api.GET("/persons/:id/follow-ups", followUpHandler.ListByPerson)

	// Reports: structured aggregation first, narration second, both persisted.
	api.POST("/reports", reportHandler.Generate)
	api.GET("/reports", reportHandler.List)
	api.GET("/reports/:id", reportHandler.Get)
	api.DELETE("/reports/:id", reportHandler.Delete)

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
