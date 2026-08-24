package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Joshua504/cineroom/internal/config"
	"github.com/Joshua504/cineroom/internal/server"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("invalid configuration: %v", err)
	}

	srv, app, err := server.New(cfg, logger)
	if err != nil {
		logger.Fatalf("starting application: %v", err)
	}
	defer app.Close()

	go func() {
		logger.Printf("starting server on %s", srv.Addr)

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	<-shutdownSignal

	logger.Println("shutdown signal received")
	app.CloseConnections()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Printf("graceful shutdown failed: %v", err)
		if closeErr := srv.Close(); closeErr != nil {
			logger.Printf("forced shutdown failed: %v", closeErr)
		}
		return
	}

	logger.Println("server stopped")
}
