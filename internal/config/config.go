package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL        string
	APIAddr            string
	ConsoleAuthEnabled bool
	APIKey             string
	WebOrigin          string
	MaxRequestBytes    int64
	RateLimitPerMin    int
	WorkerID           string
	WorkerConcurrency  int
	JobPollInterval    time.Duration
	JobLockTimeout     time.Duration
	ProviderTimeout    time.Duration
	AI                 AIConfig
	SMTP               SMTPConfig
	MailpitSMTP        SMTPConfig
	MailpitPublicURL   string
	Telegram           TelegramConfig
	Webhook            WebhookConfig
	Ingress            IngressConfig
	Connectors         ConnectorConfig
	DemoSeed           bool
}

type AIConfig struct {
	Enabled         bool
	APIKey          string
	Model           string
	Timeout         time.Duration
	MinConfidence   float64
	MaxInputChars   int
	MaxOutputTokens int
	SummaryMaxChars int
	ThinkingLevel   string
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

type IngressConfig struct {
	EncryptionKey string
}

type ConnectorConfig struct {
	PublicURL           string
	EncryptionKey       string
	TelegramAPIID       int
	TelegramAPIHash     string
	GitHubAppSlug       string
	GitHubAppID         int64
	GitHubWebhookSecret string
	GitHubPrivateKeyB64 string
	DiscordClientID     string
	DiscordClientSecret string
	DiscordBotToken     string
	GoogleClientID      string
	GoogleClientSecret  string
	GmailAPIBase        string
	CalendarAPIBase     string
	DriveActivityBase   string
	TasksAPIBase        string
	ChatAPIBase         string
	GooglePollInterval  time.Duration
	YouTubeAPIBase      string
	YouTubeFeedBase     string
	YouTubePollInterval time.Duration
}

func Load() (Config, error) {
	config := Config{
		DatabaseURL:        env("DATABASE_URL", "postgres://dispatch:dispatch@localhost:5432/dispatch?sslmode=disable"),
		APIAddr:            env("API_ADDR", ":8080"),
		ConsoleAuthEnabled: envBool("CONSOLE_AUTH_ENABLED", false),
		APIKey:             env("API_KEY", "dispatch-local-development-key"),
		WebOrigin:          env("WEB_ORIGIN", "http://localhost:5173"),
		MaxRequestBytes:    envInt64("MAX_REQUEST_BYTES", 1<<20),
		RateLimitPerMin:    envInt("RATE_LIMIT_PER_MINUTE", 120),
		WorkerID:           env("WORKER_ID", "dispatch-worker-1"),
		WorkerConcurrency:  envInt("WORKER_CONCURRENCY", 4),
		JobPollInterval:    envDuration("JOB_POLL_INTERVAL", 500*time.Millisecond),
		JobLockTimeout:     envDuration("JOB_LOCK_TIMEOUT", 2*time.Minute),
		ProviderTimeout:    envDuration("PROVIDER_TIMEOUT", 10*time.Second),
		DemoSeed:           envBool("DEMO_SEED", false),
		AI: AIConfig{
			Enabled:         envBool("AI_ENABLED", false),
			APIKey:          strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
			Model:           env("GEMINI_MODEL", "gemini-3.5-flash-lite"),
			Timeout:         envDuration("AI_TIMEOUT", 12*time.Second),
			MinConfidence:   envFloat("AI_MIN_CONFIDENCE", 0.75),
			MaxInputChars:   envInt("AI_MAX_INPUT_CHARS", 12000),
			MaxOutputTokens: envInt("AI_MAX_OUTPUT_TOKENS", 512),
			SummaryMaxChars: envInt("AI_SUMMARY_MAX_CHARS", 180),
			ThinkingLevel:   strings.ToLower(env("AI_THINKING_LEVEL", "minimal")),
			PromptVersion:   env("AI_PROMPT_VERSION", "dispatch-analysis-v3"),
		},
		SMTP: SMTPConfig{
			Host: env("SMTP_HOST", "localhost"), Port: envInt("SMTP_PORT", 1025),
			Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD"),
			From: env("SMTP_FROM", "dispatch@localhost"), TLS: envBool("SMTP_TLS", false),
		},
		MailpitSMTP: SMTPConfig{
			Host: env("MAILPIT_SMTP_HOST", "mailpit"), Port: envInt("MAILPIT_SMTP_PORT", 1025),
			From: env("MAILPIT_SMTP_FROM", "dispatch@localhost"),
		},
		MailpitPublicURL: strings.TrimRight(env("MAILPIT_PUBLIC_URL", "http://localhost:8025"), "/"),
		Telegram: TelegramConfig{
			Token:   strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
			APIBase: env("TELEGRAM_API_BASE", "https://api.telegram.org"),
		},
		Webhook: WebhookConfig{
			Enabled: envBool("WEBHOOK_ENABLED", true), DefaultURL: strings.TrimSpace(os.Getenv("WEBHOOK_DEFAULT_URL")),
			AllowPrivate: envBool("WEBHOOK_ALLOW_PRIVATE", false),
		},
		Ingress: IngressConfig{EncryptionKey: strings.TrimSpace(os.Getenv("INGRESS_ENCRYPTION_KEY"))},
		Connectors: ConnectorConfig{
			PublicURL:           strings.TrimRight(env("CONNECTOR_PUBLIC_URL", env("WEB_ORIGIN", "http://localhost:5173")), "/"),
			EncryptionKey:       strings.TrimSpace(os.Getenv("CONNECTOR_ENCRYPTION_KEY")),
			TelegramAPIID:       envInt("TELEGRAM_API_ID", 0),
			TelegramAPIHash:     strings.TrimSpace(os.Getenv("TELEGRAM_API_HASH")),
			GitHubAppSlug:       strings.TrimSpace(os.Getenv("GITHUB_APP_SLUG")),
			GitHubAppID:         envInt64("GITHUB_APP_ID", 0),
			GitHubWebhookSecret: strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET")),
			GitHubPrivateKeyB64: strings.TrimSpace(os.Getenv("GITHUB_PRIVATE_KEY_BASE64")),
			DiscordClientID:     strings.TrimSpace(os.Getenv("DISCORD_CLIENT_ID")),
			DiscordClientSecret: strings.TrimSpace(os.Getenv("DISCORD_CLIENT_SECRET")),
			DiscordBotToken:     strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
			GoogleClientID:      strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_ID")),
			GoogleClientSecret:  strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")),
			GmailAPIBase:        env("GOOGLE_GMAIL_API_BASE", "https://gmail.googleapis.com"),
			CalendarAPIBase:     env("GOOGLE_CALENDAR_API_BASE", "https://www.googleapis.com/calendar/v3"),
			DriveActivityBase:   env("GOOGLE_DRIVE_ACTIVITY_API_BASE", "https://driveactivity.googleapis.com/v2"),
			TasksAPIBase:        env("GOOGLE_TASKS_API_BASE", "https://tasks.googleapis.com/tasks/v1"),
			ChatAPIBase:         env("GOOGLE_CHAT_API_BASE", "https://chat.googleapis.com/v1"),
			GooglePollInterval:  envDuration("GOOGLE_POLL_INTERVAL", envDuration("GMAIL_POLL_INTERVAL", 30*time.Second)),
			YouTubeAPIBase:      env("YOUTUBE_API_BASE", "https://www.googleapis.com/youtube/v3"),
			YouTubeFeedBase:     env("YOUTUBE_FEED_BASE", "https://www.youtube.com"),
			YouTubePollInterval: envDuration("YOUTUBE_POLL_INTERVAL", 10*time.Minute),
		},
	}
	if config.Ingress.EncryptionKey == "" {
		config.Ingress.EncryptionKey = config.APIKey
	}
	if config.Connectors.EncryptionKey == "" {
		config.Connectors.EncryptionKey = config.Ingress.EncryptionKey
	}
	if config.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if config.ConsoleAuthEnabled && (config.APIKey == "" || len(config.APIKey) < 16) {
		return Config{}, fmt.Errorf("API_KEY must contain at least 16 characters")
	}
	if config.AI.Enabled && config.AI.APIKey == "" {
		return Config{}, fmt.Errorf("GEMINI_API_KEY is required when AI_ENABLED=true")
	}
	if config.AI.MinConfidence < 0 || config.AI.MinConfidence > 1 {
		return Config{}, fmt.Errorf("AI_MIN_CONFIDENCE must be between 0 and 1")
	}
	if config.AI.MaxInputChars < 1000 || config.AI.MaxInputChars > 100000 {
		return Config{}, fmt.Errorf("AI_MAX_INPUT_CHARS must be between 1000 and 100000")
	}
	if config.AI.MaxOutputTokens < 128 || config.AI.MaxOutputTokens > 4096 {
		return Config{}, fmt.Errorf("AI_MAX_OUTPUT_TOKENS must be between 128 and 4096")
	}
	if config.AI.SummaryMaxChars < 80 || config.AI.SummaryMaxChars > 500 {
		return Config{}, fmt.Errorf("AI_SUMMARY_MAX_CHARS must be between 80 and 500")
	}
	if config.AI.ThinkingLevel != "minimal" && config.AI.ThinkingLevel != "low" &&
		config.AI.ThinkingLevel != "medium" && config.AI.ThinkingLevel != "high" {
		return Config{}, fmt.Errorf("AI_THINKING_LEVEL must be minimal, low, medium, or high")
	}
	if config.WorkerConcurrency < 1 || config.WorkerConcurrency > 64 {
		return Config{}, fmt.Errorf("WORKER_CONCURRENCY must be between 1 and 64")
	}
	if config.Connectors.GooglePollInterval < 5*time.Second {
		return Config{}, fmt.Errorf("GOOGLE_POLL_INTERVAL must be at least 5s")
	}
	if config.Connectors.YouTubePollInterval < time.Minute {
		return Config{}, fmt.Errorf("YOUTUBE_POLL_INTERVAL must be at least 1m")
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
