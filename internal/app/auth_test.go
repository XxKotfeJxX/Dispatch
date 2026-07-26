package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"dispatch/internal/config"
)

func TestAuthenticateBypassesKeyInLocalMode(t *testing.T) {
	api := API{Config: config.Config{ConsoleAuthEnabled: false}}
	handler := api.authenticate(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestAuthenticateRequiresConfiguredKeyWhenEnabled(t *testing.T) {
	api := API{Config: config.Config{
		ConsoleAuthEnabled: true,
		APIKey:             "a-long-console-api-key",
	}}
	handler := api.authenticate(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))

	t.Run("missing key", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
		}
	})

	t.Run("valid key", func(t *testing.T) {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
		request.Header.Set("X-API-Key", "a-long-console-api-key")
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
		}
	})
}
