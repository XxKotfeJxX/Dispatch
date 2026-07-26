package connectors

import (
	"net/url"
	"strings"

	"dispatch/internal/config"
)

func Catalog(cfg config.ConnectorConfig) []Manifest {
	publicHTTPS := strings.HasPrefix(strings.ToLower(cfg.PublicURL), "https://")
	googleReady := cfg.GoogleClientID != "" && cfg.GoogleClientSecret != ""
	githubReady := cfg.GitHubAppSlug != "" && cfg.GitHubAppID > 0 &&
		cfg.GitHubWebhookSecret != "" && cfg.GitHubPrivateKeyB64 != ""
	githubHint := "Set the GitHub App slug, App ID, webhook secret, and private key."
	if githubReady {
		githubHint = "GitHub App is configured for one-click installation."
		if !publicHTTPS {
			githubHint = "Installation is configured; live events require public HTTPS or the development webhook proxy."
		}
	}
	discordReady := cfg.DiscordClientID != "" && cfg.DiscordClientSecret != "" &&
		cfg.DiscordBotToken != ""
	telegramReady := cfg.TelegramAPIID > 0 && cfg.TelegramAPIHash != ""
	result := []Manifest{
		{
			ID: "demo", Name: "Demo source", Category: "Testing",
			Summary: "Generate a safe sample event without an external account.",
			Auth:    AuthNone, Transport: TransportInternal, Availability: Available, Configured: true,
			Capabilities: []string{"sample events", "delivery verification"},
		},
		{
			ID: "telegram", Name: "Telegram", Category: "Messaging",
			Summary: "Connect your personal Telegram account and receive new message events.",
			Auth:    AuthUserSession, Transport: TransportGateway, Availability: SetupRequired, Configured: telegramReady,
			Capabilities: []string{"personal account", "private chats", "groups", "channels", "real-time updates"},
			Fields: []Field{{
				Name: "phone_number", Label: "Phone number", Type: "tel", Required: true,
				Placeholder: "+380…", Help: "Telegram sends the sign-in code to your existing Telegram session or phone.",
			}},
			SetupHint: availabilityHint(telegramReady,
				"Ready for secure phone, code, and optional 2FA authorization.",
				"Telegram account connections are not enabled on this installation yet."),
			Documentation: "https://core.telegram.org/api/auth",
		},
		{
			ID: "discord", Name: "Discord", Category: "Messaging",
			Summary: "Authorize Dispatch, choose a server, and receive the message types you select.",
			Auth:    AuthOAuth2, Transport: TransportGateway, Availability: SetupRequired, Configured: discordReady,
			Capabilities: []string{"bot installation", "direct messages", "server channels", "Gateway"},
			SetupHint: availabilityHint(discordReady,
				"Ready for one-click Discord authorization and server installation.",
				"Set DISCORD_CLIENT_ID, DISCORD_CLIENT_SECRET, and DISCORD_BOT_TOKEN."),
			Documentation: "https://docs.discord.com/developers/topics/oauth2",
		},
		{
			ID: "github", Name: "GitHub", Category: "Development",
			Summary: "Install a GitHub App once and receive repository or organization events.",
			Auth:    AuthAppInstall, Transport: TransportWebhook, Availability: SetupRequired, Configured: githubReady,
			Capabilities:  []string{"repository events", "issues", "pull requests", "workflow events"},
			SetupHint:     githubHint,
			Documentation: "https://docs.github.com/en/apps/creating-github-apps",
		},
		{
			ID: "google", Name: "Google", Category: "Productivity",
			Summary: "Connect Google once for Gmail, Calendar, Meet, Drive, Tasks, and Chat.",
			Auth:    AuthOAuth2, Transport: TransportPolling, Availability: SetupRequired, Configured: googleReady,
			Capabilities: []string{"Gmail", "Calendar and Meet", "Drive and editors", "Tasks", "Google Chat"},
			SetupHint: availabilityHint(googleReady,
				"Ready for one Google authorization and modular background synchronization.",
				"Set GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET."),
			Documentation: "https://developers.google.com/workspace",
		},
		{
			ID: "youtube", Name: "YouTube", Category: "Media",
			Summary: "Sign in once and receive new videos from every channel you subscribe to.",
			Auth:    AuthOAuth2, Transport: TransportPolling, Availability: SetupRequired, Configured: googleReady,
			Capabilities: []string{
				"personal subscriptions", "new uploads", "automatic channel sync", "local polling",
			},
			SetupHint: availabilityHint(googleReady,
				"Ready to read YouTube subscriptions with Google authorization.",
				"Set GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET."),
			Documentation: "https://developers.google.com/youtube/v3/docs/subscriptions/list",
		},
		{
			ID: "webhook", Name: "Universal webhook", Category: "Advanced",
			Summary: "Connect any JSON-producing service with a source-specific endpoint and secret.",
			Auth:    AuthAPIKey, Transport: TransportWebhook, Availability: Available, Configured: true,
			Capabilities:  []string{"presets", "HMAC", "JSON mapping", "deduplication"},
			SetupHint:     "Use the advanced webhook builder when a managed connector is unavailable.",
			Documentation: "https://github.com/XxKotfeJxX/Dispatch/blob/dev/docs/ingress.md",
		},
	}
	for index := range result {
		if result[index].Fields == nil {
			result[index].Fields = []Field{}
		}
	}
	return result
}

func Find(cfg config.ConnectorConfig, id string) (Manifest, bool) {
	for _, manifest := range Catalog(cfg) {
		if manifest.ID == id {
			return manifest, true
		}
	}
	return Manifest{}, false
}

func availabilityHint(configured bool, ready, missing string) string {
	if configured {
		return ready
	}
	return missing
}

func GitHubInstallURL(slug, state string) string {
	if slug == "" {
		return ""
	}
	target := "https://github.com/apps/" + url.PathEscape(slug) + "/installations/new"
	if state != "" {
		target += "?state=" + url.QueryEscape(state)
	}
	return target
}

func OAuthSpec(id string, cfg config.ConnectorConfig) (OAuthProvider, bool) {
	if id == "discord" && cfg.DiscordClientID != "" && cfg.DiscordClientSecret != "" &&
		cfg.DiscordBotToken != "" {
		return OAuthProvider{
			ID: id, ClientID: cfg.DiscordClientID, ClientSecret: cfg.DiscordClientSecret,
			AuthorizeURL: "https://discord.com/oauth2/authorize",
			TokenURL:     "https://discord.com/api/v10/oauth2/token",
			UserInfoURL:  "https://discord.com/api/v10/users/@me",
			RevokeURL:    "https://discord.com/api/v10/oauth2/token/revoke",
			Scopes:       []string{"identify", "bot", "applications.commands"},
			ExtraAuthorize: map[string]string{
				"integration_type": "0",
				"permissions":      "68608",
				"prompt":           "consent",
			},
		}, true
	}
	if (id != "google" && id != "youtube") ||
		cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		return OAuthProvider{}, false
	}
	scopes := []string{"openid", "email"}
	if id == "youtube" {
		scopes = append(scopes, "https://www.googleapis.com/auth/youtube.readonly")
	}
	return OAuthProvider{
		ID: id, ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret,
		AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
		RevokeURL:    "https://oauth2.googleapis.com/revoke",
		Scopes:       scopes,
		ExtraAuthorize: map[string]string{
			"access_type": "offline", "include_granted_scopes": "true", "prompt": "consent",
		},
	}, true
}

type OAuthProvider struct {
	ID             string
	ClientID       string
	ClientSecret   string
	AuthorizeURL   string
	TokenURL       string
	UserInfoURL    string
	RevokeURL      string
	Scopes         []string
	ExtraAuthorize map[string]string
}
