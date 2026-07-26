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

func TestTelegramMessageMetadataIncludesSenderAndPrivateChat(t *testing.T) {
	user := &tg.User{
		ID: 42, FirstName: "Ada", LastName: "Lovelace", Username: "ada",
		Premium: true,
	}
	metadata := telegramMessageMetadata(tg.Entities{
		Users: map[int64]*tg.User{42: user},
	}, &tg.Message{
		ID: 7, Date: 1785085200, Message: "Hello",
		FromID:    &tg.PeerUser{UserID: 42},
		PeerID:    &tg.PeerUser{UserID: 42},
		Mentioned: true,
	})
	if metadata["sender"] != "Ada Lovelace" ||
		metadata["sender_username"] != "ada" ||
		metadata["chat_title"] != "Ada Lovelace" ||
		metadata["chat_type"] != "private" ||
		metadata["mentioned"] != true {
		t.Fatalf("incomplete Telegram metadata: %#v", metadata)
	}
}
