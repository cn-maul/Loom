// Package main is a thin launcher. All dependency assembly lives in
// internal/app; this file only parses arguments, starts the server and shuts
// it down.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"relationship/internal/app"
	"relationship/internal/config"
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

	application, err := app.New(cfg, cfgPath)
	if err != nil {
		log.Fatalf("start app: %v", err)
	}
	defer application.Close()

	application.WarnIfExposed()

	e, err := application.NewRouter()
	if err != nil {
		log.Printf("warning: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("server starting on http://%s", addr)

	errCh := make(chan error, 1)
	go func() {
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Graceful shutdown: stop accepting requests, then close the database.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		log.Fatalf("server error: %v", err)
	case sig := <-sigCh:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}
}
