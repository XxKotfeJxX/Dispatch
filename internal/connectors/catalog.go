package connectors

import (
	"net/url"
	"strings"

	"dispatch/internal/config"
)

func Catalog(cfg config.ConnectorConfig) []Manifest {
	publicHTTPS := strings.HasPrefix(strings.ToLower(cfg.PublicURL), "https://")
	googleReady := cfg.GoogleClientID != "" && cfg.GoogleClientSecret != ""
	githubReady := cfg.GitHubAppSlug != ""
	discordReady := cfg.DiscordClientID != ""
	result := []Manifest{
		{
			ID: "demo", Name: "Demo source", Category: "Testing",
			Summary: "Generate a safe sample event without an external account.",
			Auth:    AuthNone, Transport: TransportInternal, Availability: Available, Configured: true,
			Capabilities: []string{"sample events", "delivery verification"},
		},
		{
			ID: "telegram", Name: "Telegram", Category: "Messaging",
			Summary: "Receive private bot messages and route them through Dispatch.",
			Auth:    AuthBotToken, Transport: TransportWebhook, Availability: Available, Configured: publicHTTPS,
			Capabilities: []string{"bot messages", "automatic webhook", "sender identity"},
			Fields: []Field{{
				Name: "bot_token", Label: "Bot token", Type: "password", Required: true, Secret: true,
				Placeholder: "123456:ABC…", Help: "Create a bot with @BotFather and paste its token.",
			}},
			SetupHint: availabilityHint(publicHTTPS,
				"Ready for one-click webhook registration.",
				"Token verification works locally; receiving events requires CONNECTOR_PUBLIC_URL with public HTTPS."),
			Documentation: "https://core.telegram.org/bots/api",
		},
		{
			ID: "discord", Name: "Discord", Category: "Messaging",
			Summary: "Install the Dispatch bot and receive allowlisted Gateway events.",
			Auth:    AuthAppInstall, Transport: TransportGateway, Availability: SetupRequired, Configured: discordReady,
			Capabilities: []string{"bot installation", "direct messages", "server channels", "Gateway"},
			SetupHint: availabilityHint(discordReady,
				"Discord application is configured; install the bot, then run the Gateway bridge.",
				"Set DISCORD_CLIENT_ID and the existing Discord bridge credentials first."),
			Documentation:    "https://docs.discord.com/developers/topics/oauth2",
			AuthorizationURL: discordInstallURL(cfg.DiscordClientID),
		},
		{
			ID: "viber", Name: "Viber", Category: "Messaging",
			Summary: "Receive messages from a commercially provisioned Viber bot.",
			Auth:    AuthBotToken, Transport: TransportWebhook, Availability: Limited, Configured: publicHTTPS,
			Capabilities: []string{"bot messages", "automatic webhook", "delivery events"},
			Fields: []Field{{
				Name: "auth_token", Label: "Viber bot token", Type: "password", Required: true, Secret: true,
				Help: "Viber has required commercial chatbot provisioning since February 2024.",
			}},
			SetupHint: availabilityHint(publicHTTPS,
				"Commercial bot token required; Dispatch can register its webhook.",
				"Commercial bot token and a public HTTPS CONNECTOR_PUBLIC_URL are required."),
			Documentation: "https://developers.viber.com/docs/api/rest-bot-api/",
		},
		{
			ID: "github", Name: "GitHub", Category: "Development",
			Summary: "Install a GitHub App once and receive repository or organization events.",
			Auth:    AuthAppInstall, Transport: TransportWebhook, Availability: SetupRequired, Configured: githubReady,
			Capabilities: []string{"repository events", "issues", "pull requests", "workflow events"},
			SetupHint: availabilityHint(githubReady,
				"GitHub App is configured and ready to install.",
				"Create a GitHub App and set GITHUB_APP_SLUG; universal GitHub webhooks remain available."),
			Documentation:    "https://docs.github.com/en/apps/creating-github-apps",
			AuthorizationURL: githubInstallURL(cfg.GitHubAppSlug),
		},
		{
			ID: "google", Name: "Google / Gmail", Category: "Productivity",
			Summary: "Authorize a Google account for Gmail change notifications.",
			Auth:    AuthOAuth2, Transport: TransportWebhook, Availability: SetupRequired, Configured: googleReady,
			Capabilities: []string{"OAuth 2.0", "Gmail mailbox watch", "token refresh"},
			SetupHint: availabilityHint(googleReady,
				"OAuth is configured; Gmail Pub/Sub provisioning is the next activation step.",
				"Set GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET."),
			Documentation: "https://developers.google.com/workspace/gmail/api/guides/push",
		},
		{
			ID: "youtube", Name: "YouTube", Category: "Media",
			Summary: "Watch a channel for uploads and metadata changes through WebSub.",
			Auth:    AuthNone, Transport: TransportWebSub, Availability: Available, Configured: publicHTTPS,
			Capabilities: []string{"new uploads", "title changes", "description changes", "WebSub"},
			Fields: []Field{{
				Name: "channel_id", Label: "Channel ID", Type: "text", Required: true,
				Placeholder: "UC…", Help: "The channel whose public video feed should be monitored.",
			}},
			SetupHint: availabilityHint(publicHTTPS,
				"Ready to create a WebSub subscription.",
				"Connection can be saved locally; live events require public HTTPS."),
			Documentation: "https://developers.google.com/youtube/v3/guides/push_notifications",
		},
		{
			ID: "chatgpt", Name: "ChatGPT", Category: "AI",
			Summary: "Consumer ChatGPT notification subscriptions are not exposed as a public API.",
			Auth:    AuthOAuth2, Transport: TransportWebhook, Availability: Unavailable, Configured: false,
			Capabilities:  []string{"OpenAI API webhooks are a separate future connector"},
			SetupHint:     "ChatGPT Apps connect ChatGPT to external tools; they do not export a user's ChatGPT notifications.",
			Documentation: "https://developers.openai.com/apps-sdk/",
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

func githubInstallURL(slug string) string {
	if slug == "" {
		return ""
	}
	return "https://github.com/apps/" + url.PathEscape(slug) + "/installations/new"
}

func discordInstallURL(clientID string) string {
	if clientID == "" {
		return ""
	}
	query := url.Values{
		"client_id":        {clientID},
		"scope":            {"bot applications.commands"},
		"permissions":      {"274877975552"},
		"integration_type": {"0"},
	}
	return "https://discord.com/oauth2/authorize?" + query.Encode()
}

func OAuthSpec(id string, cfg config.ConnectorConfig) (OAuthProvider, bool) {
	if id != "google" || cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		return OAuthProvider{}, false
	}
	return OAuthProvider{
		ID: id, ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret,
		AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
		RevokeURL:    "https://oauth2.googleapis.com/revoke",
		Scopes: []string{
			"openid", "email",
			"https://www.googleapis.com/auth/gmail.readonly",
		},
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
