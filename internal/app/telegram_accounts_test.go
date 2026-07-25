package app

import (
	"bytes"
	"context"
	"testing"

	"github.com/gotd/td/tg"
)

func TestTelegramMemorySessionCopiesSensitiveBytes(t *testing.T) {
	storage := &telegramMemorySession{}
	original := []byte("session")
	if err := storage.StoreSession(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	original[0] = 'X'
	loaded, err := storage.LoadSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, []byte("session")) {
		t.Fatalf("stored session was mutated: %q", loaded)
	}
	loaded[0] = 'Y'
	if !bytes.Equal(storage.Bytes(), []byte("session")) {
		t.Fatal("loaded session exposed internal storage")
	}
}

func TestTelegramAccountHelpers(t *testing.T) {
	for _, value := range []string{"+380501234567", "+12025550123"} {
		if !telegramPhonePattern.MatchString(value) {
			t.Fatalf("valid phone rejected: %s", value)
		}
	}
	for _, value := range []string{"380501234567", "+12", "+012345678"} {
		if telegramPhonePattern.MatchString(value) {
			t.Fatalf("invalid phone accepted: %s", value)
		}
	}
	if got := telegramUserLabel(&tg.User{Username: "ada"}); got != "@ada" {
		t.Fatalf("label=%q", got)
	}
	if got := telegramUserLabel(&tg.User{FirstName: "Ada", LastName: "Lovelace"}); got != "Ada Lovelace" {
		t.Fatalf("label=%q", got)
	}
}
