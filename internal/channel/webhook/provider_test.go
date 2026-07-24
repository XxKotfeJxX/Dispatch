package webhook

import (
	"context"
	"net"
	"net/http"
	"testing"

	"dispatch/internal/config"
)

func TestValidateURLBlocksPrivateAndUnsafeDestinations(t *testing.T) {
	provider := New(config.WebhookConfig{Enabled: true}, http.DefaultClient)
	provider.lookup = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }
	for _, value := range []string{"http://localhost/hook", "http://user:pass@example.test/hook", "file:///etc/passwd"} {
		if err := provider.ValidateURL(context.Background(), value); err == nil {
			t.Errorf("expected %q to be rejected", value)
		}
	}
}

func TestValidateURLAllowsPublicDestination(t *testing.T) {
	provider := New(config.WebhookConfig{Enabled: true}, http.DefaultClient)
	provider.lookup = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("203.0.113.20")}, nil }
	if err := provider.ValidateURL(context.Background(), "https://hooks.example.test/dispatch"); err != nil {
		t.Fatal(err)
	}
}
