package connectors

import (
	"net/url"
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
		byID["google"].Configured || byID["github"].Configured {
		t.Fatalf("unexpected catalog state: %#v", byID)
	}
	for _, removed := range []string{"telegram", "viber", "chatgpt"} {
		if _, exists := byID[removed]; exists {
			t.Fatalf("%s must not be offered as a consumer connector", removed)
		}
	}
}
