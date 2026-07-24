package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"dispatch/internal/ai"
	"dispatch/internal/app"
	"dispatch/internal/channel/email"
	"dispatch/internal/channel/telegram"
	"dispatch/internal/channel/webhook"
	"dispatch/internal/config"
	"dispatch/internal/delivery"
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
	client := &http.Client{}
	var decisionProvider ai.DecisionProvider
	if cfg.AI.Enabled {
		decisionProvider = ai.NewGemini(cfg.AI, client)
	}
	worker := app.Worker{
		Config: cfg, Store: store, AI: decisionProvider, Logger: slog.Default(),
		Providers: map[delivery.Channel]delivery.Provider{
			delivery.ChannelEmail:    email.New(cfg.SMTP),
			delivery.ChannelTelegram: telegram.New(cfg.Telegram, client),
			delivery.ChannelWebhook:  webhook.New(cfg.Webhook, client),
		},
	}
	if err := worker.Run(ctx); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
