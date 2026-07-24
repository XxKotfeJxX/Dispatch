package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL       string
	APIAddr           string
	APIKey            string
	WebOrigin         string
	MaxRequestBytes   int64
	RateLimitPerMin   int
	WorkerID          string
	WorkerConcurrency int
	JobPollInterval   time.Duration
	JobLockTimeout    time.Duration
	ProviderTimeout   time.Duration
	AI                AIConfig
	SMTP              SMTPConfig
	Telegram          TelegramConfig
	Webhook           WebhookConfig
	DemoSeed          bool
}

type AIConfig struct {
	Enabled         bool
	APIKey          string
	Model           string
	Timeout         time.Duration
	MinConfidence   float64
	MaxOutputTokens int
	PromptVersion   string
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLS      bool
}

type TelegramConfig struct {
	Token   string
	APIBase string
}

type WebhookConfig struct {
	Enabled      bool
	DefaultURL   string
	AllowPrivate bool
}

func Load() (Config, error) {
	config := Config{
		DatabaseURL:       env("DATABASE_URL", "postgres://dispatch:dispatch@localhost:5432/dispatch?sslmode=disable"),
		APIAddr:           env("API_ADDR", ":8080"),
		APIKey:            env("API_KEY", "dispatch-local-development-key"),
		WebOrigin:         env("WEB_ORIGIN", "http://localhost:5173"),
		MaxRequestBytes:   envInt64("MAX_REQUEST_BYTES", 1<<20),
		RateLimitPerMin:   envInt("RATE_LIMIT_PER_MINUTE", 120),
		WorkerID:          env("WORKER_ID", "dispatch-worker-1"),
		WorkerConcurrency: envInt("WORKER_CONCURRENCY", 4),
		JobPollInterval:   envDuration("JOB_POLL_INTERVAL", 500*time.Millisecond),
		JobLockTimeout:    envDuration("JOB_LOCK_TIMEOUT", 2*time.Minute),
		ProviderTimeout:   envDuration("PROVIDER_TIMEOUT", 10*time.Second),
		DemoSeed:          envBool("DEMO_SEED", false),
		AI: AIConfig{
			Enabled:         envBool("AI_ENABLED", false),
			APIKey:          strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
			Model:           env("GEMINI_MODEL", "gemini-3.5-flash-lite"),
			Timeout:         envDuration("AI_TIMEOUT", 5*time.Second),
			MinConfidence:   envFloat("AI_MIN_CONFIDENCE", 0.75),
			MaxOutputTokens: envInt("AI_MAX_OUTPUT_TOKENS", 512),
			PromptVersion:   env("AI_PROMPT_VERSION", "dispatch-routing-v1"),
		},
		SMTP: SMTPConfig{
			Host: env("SMTP_HOST", "localhost"), Port: envInt("SMTP_PORT", 1025),
			Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD"),
			From: env("SMTP_FROM", "dispatch@localhost"), TLS: envBool("SMTP_TLS", false),
		},
		Telegram: TelegramConfig{
			Token:   strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
			APIBase: env("TELEGRAM_API_BASE", "https://api.telegram.org"),
		},
		Webhook: WebhookConfig{
			Enabled: envBool("WEBHOOK_ENABLED", true), DefaultURL: strings.TrimSpace(os.Getenv("WEBHOOK_DEFAULT_URL")),
			AllowPrivate: envBool("WEBHOOK_ALLOW_PRIVATE", false),
		},
	}
	if config.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if config.APIKey == "" || len(config.APIKey) < 16 {
		return Config{}, fmt.Errorf("API_KEY must contain at least 16 characters")
	}
	if config.AI.Enabled && config.AI.APIKey == "" {
		return Config{}, fmt.Errorf("GEMINI_API_KEY is required when AI_ENABLED=true")
	}
	if config.WorkerConcurrency < 1 || config.WorkerConcurrency > 64 {
		return Config{}, fmt.Errorf("WORKER_CONCURRENCY must be between 1 and 64")
	}
	return config, nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}

func envInt64(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func envFloat(name string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(name)), 64)
	if err != nil {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}
