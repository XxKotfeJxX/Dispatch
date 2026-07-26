package connectors

import (
	"net/url"
	"strings"
	"testing"
)

func TestOAuthValuesUsesStateAndPKCE(t *testing.T) {
	provider := OAuthProvider{
		ClientID: "client", AuthorizeURL: "https://provider.example/authorize",
		Scopes:         []string{"openid", "events.read"},
		ExtraAuthorize: map[string]string{"access_type": "offline"},
	}
	raw := OAuthValues(provider, "https://dispatch.example/callback", "state-value", "verifier-value")
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("state") != "state-value" || query.Get("code_challenge") == "" ||
		query.Get("code_challenge_method") != "S256" || query.Get("access_type") != "offline" {
		t.Fatalf("unexpected OAuth URL: %s", raw)
	}
}

func TestCatalogReportsDeploymentReadiness(t *testing.T) {
	items := Catalog(testConnectorConfig())
	byID := map[string]Manifest{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if !byID["demo"].Configured || byID["discord"].Configured ||
		byID["google"].Configured || byID["youtube"].Configured ||
		byID["github"].Configured {
		t.Fatalf("unexpected catalog state: %#v", byID)
	}
	if byID["telegram"].Configured || byID["telegram"].Auth != AuthUserSession {
		t.Fatalf("Telegram must use a deployment-configured user session: %#v", byID["telegram"])
	}
	for _, removed := range []string{"viber", "chatgpt"} {
		if _, exists := byID[removed]; exists {
			t.Fatalf("%s must not be offered as a consumer connector", removed)
		}
	}
}

func TestYouTubeUsesGoogleOAuthWithReadOnlyScope(t *testing.T) {
	cfg := testConnectorConfig()
	cfg.GoogleClientID = "google-client"
	cfg.GoogleClientSecret = "google-secret"

	manifest, ok := Find(cfg, "youtube")
	if !ok || !manifest.Configured || manifest.Auth != AuthOAuth2 ||
		manifest.Transport != TransportPolling || len(manifest.Fields) != 0 {
		t.Fatalf("YouTube must use managed polling OAuth: %#v", manifest)
	}
	provider, ok := OAuthSpec("youtube", cfg)
	if !ok || !strings.Contains(
		strings.Join(provider.Scopes, " "),
		"https://www.googleapis.com/auth/youtube.readonly",
	) {
		t.Fatalf("YouTube OAuth scope missing: %#v", provider)
	}
}

func TestDiscordOAuthRequiresCompleteDeploymentConfiguration(t *testing.T) {
	cfg := testConnectorConfig()
	cfg.DiscordClientID = "client"
	cfg.DiscordClientSecret = "secret"
	cfg.DiscordBotToken = "token"

	manifest, ok := Find(cfg, "discord")
	if !ok || !manifest.Configured || manifest.Auth != AuthOAuth2 {
		t.Fatalf("Discord must be a configured managed OAuth connector: %#v", manifest)
	}
	provider, ok := OAuthSpec("discord", cfg)
	if !ok {
		t.Fatal("Discord OAuth provider was not created")
	}
	raw := OAuthValues(
		provider,
		"http://localhost:8090/connect/v1/oauth/discord/callback",
		"state",
		"verifier",
	)
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("permissions") != "68608" ||
		!strings.Contains(query.Get("scope"), "bot") ||
		!strings.Contains(query.Get("scope"), "identify") {
		t.Fatalf("unexpected Discord authorization URL: %s", raw)
	}
}

func TestGitHubCatalogRequiresSecureAppConfiguration(t *testing.T) {
	cfg := testConnectorConfig()
	cfg.GitHubAppSlug = "dispatch"
	cfg.GitHubAppID = 42
	cfg.GitHubWebhookSecret = "webhook-secret"
	cfg.GitHubPrivateKeyB64 = "private-key"

	manifest, ok := Find(cfg, "github")
	if !ok || !manifest.Configured || manifest.Auth != AuthAppInstall {
		t.Fatalf("GitHub must use the managed app installation flow: %#v", manifest)
	}
	installURL := GitHubInstallURL(cfg.GitHubAppSlug, "state-value")
	parsed, err := url.Parse(installURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("state") != "state-value" {
		t.Fatalf("GitHub installation URL lost state: %s", installURL)
	}
}
