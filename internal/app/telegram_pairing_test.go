package app

import "testing"

func TestTelegramStartCode(t *testing.T) {
	if got := telegramStartCode("/start abc_123"); got != "abc_123" {
		t.Fatalf("code = %q", got)
	}
	if got := telegramStartCode("hello"); got != "" {
		t.Fatalf("unexpected code %q", got)
	}
}
