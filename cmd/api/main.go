package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dispatch/internal/ai"
	"dispatch/internal/app"
	"dispatch/internal/config"
	"dispatch/internal/store/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if cfg.DemoSeed {
		if err := store.SeedDemo(ctx); err != nil {
			slog.Error("seed demo", "error", err)
			os.Exit(1)
		}
	}
	var decisionProvider ai.DecisionProvider
	if cfg.AI.Enabled {
		decisionProvider = ai.NewGemini(cfg.AI, &http.Client{})
	}
	server := &http.Server{
		Addr: cfg.APIAddr, Handler: (&app.API{Config: cfg, Store: store, AI: decisionProvider, Logger: slog.Default()}).Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	slog.Info("API listening", "address", cfg.APIAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("API stopped", "error", err)
		os.Exit(1)
	}
}
