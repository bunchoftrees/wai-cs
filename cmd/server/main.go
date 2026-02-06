package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/workforce-ai/site-selection-iq/internal/api"
	"github.com/workforce-ai/site-selection-iq/internal/config"
	"github.com/workforce-ai/site-selection-iq/internal/db"
)

func main() {
	// Initialize structured JSON logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting site-selection-iq service")

	// Load configuration
	cfg := config.Load()

	// Create temp upload directory
	if err := os.MkdirAll(cfg.Upload.TempDir, 0755); err != nil {
		slog.Error("failed to create upload temp dir", "error", err)
		os.Exit(1)
	}

	// Connect to database with retry
	ctx := context.Background()
	var dbPool = connectWithRetry(ctx, cfg, 30)
	defer dbPool.Close()

	// Run migrations
	if err := db.RunMigrations(ctx, dbPool); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Initialize router with all dependencies
	router := api.NewRouter(dbPool, cfg)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		slog.Info("server listening",
			"port", cfg.Server.Port,
			"service", "site-selection-iq",
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced shutdown", "error", err)
	}
	slog.Info("server exited")
}

func connectWithRetry(ctx context.Context, cfg *config.Config, maxRetries int) *db.Pool {
	for i := 0; i < maxRetries; i++ {
		pool, err := db.Connect(ctx, cfg.Database)
		if err == nil {
			return pool
		}
		slog.Warn("database not ready, retrying...",
			"attempt", i+1,
			"max_retries", maxRetries,
			"error", err,
		)
		time.Sleep(2 * time.Second)
	}
	slog.Error("failed to connect to database after retries")
	os.Exit(1)
	return nil
}
