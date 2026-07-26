package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dispatch/internal/config"
)

func TestTelegramBotUsername(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/botsecret/getMe" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":{"username":"DispatchTestBot"}}`))
	}))
	defer server.Close()

	api := API{Config: config.Config{
		ProviderTimeout: 5 * time.Second,
		Telegram:        config.TelegramConfig{Token: "secret", APIBase: server.URL},
	}}
	username, err := api.telegramBotUsername(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if username != "DispatchTestBot" {
		t.Fatalf("username = %q", username)
	}
}

func TestNumericCodeHasSixDigits(t *testing.T) {
	code, err := numericCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("code = %q", code)
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			t.Fatalf("code = %q", code)
		}
	}
}
