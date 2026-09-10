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

	if err := vecRepo.Init(cfg.LLM.EmbedDim); err != nil {
		log.Printf("warning: init vec_repo: %v", err)
	}

	aiClient, err := ai.NewClient(&cfg.LLM)
	if err != nil {
		log.Printf("warning: init ai client: %v", err)
	}

	personService := service.NewPersonService(personRepo)
	eventService := service.NewEventService(eventRepo)
	aiService := service.NewAIService(&cfg.LLM, vecRepo, traitRepo, eventRepo, aiClient)

	personHandler := handler.NewPersonHandler(personService)
	eventHandler := handler.NewEventHandler(eventService, aiService)
	traitHandler := handler.NewTraitHandler(traitRepo)
	aiHandler := handler.NewAIHandler(aiService)
	configHandler := handler.NewConfigHandler(cfg, cfgPath)

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// API routes
	api := e.Group("/api")
	api.POST("/persons", personHandler.Create)
	api.GET("/persons", personHandler.List)
	api.GET("/persons/:id", personHandler.GetByID)
	api.PUT("/persons/:id", personHandler.Update)
	api.DELETE("/persons/:id", personHandler.Delete)
	api.POST("/events", eventHandler.Create)
	api.GET("/events/:id", eventHandler.GetByID)
	api.DELETE("/events/:id", eventHandler.Delete)
	api.GET("/persons/:id/events", eventHandler.ListByPerson)
	api.GET("/persons/:id/traits", traitHandler.ListByPerson)
	api.PUT("/traits/:id/verify", traitHandler.Verify)
	api.POST("/ai/advice", aiHandler.GetAdvice)
	api.GET("/ai/report/weekly", aiHandler.GetWeeklyReport)
	api.GET("/config", configHandler.Get)
	api.PUT("/config", configHandler.Update)

	// Serve frontend
	frontendFS, err := frontend.GetFS()
	if err != nil {
		log.Printf("warning: load frontend: %v", err)
	} else {
		e.GET("/*", func(c echo.Context) error {
			path := c.Request().URL.Path
			if path == "/" {
				path = "index.html"
			} else {
				path = path[1:] // remove leading /
			}

			data, err := fs.ReadFile(frontendFS, path)
			if err != nil {
				// SPA fallback: serve index.html
				data, err = fs.ReadFile(frontendFS, "index.html")
				if err != nil {
					return c.String(http.StatusNotFound, "Not Found")
				}
				return c.HTML(http.StatusOK, string(data))
			}

			// detect content type
			ct := http.DetectContentType(data)
			if strings.HasSuffix(path, ".js") {
				ct = "application/javascript"
			} else if strings.HasSuffix(path, ".css") {
				ct = "text/css"
			} else if strings.HasSuffix(path, ".html") {
				ct = "text/html"
			} else if strings.HasSuffix(path, ".svg") {
				ct = "image/svg+xml"
			}
			c.Response().Header().Set(echo.HeaderContentType, ct)
			return c.Blob(http.StatusOK, ct, data)
		})
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("server starting on %s", addr)
	if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}