package email

import (
	"slices"
	"testing"

	"dispatch/internal/delivery"
)

func TestMessageTags(t *testing.T) {
	message := delivery.Message{Metadata: map[string]any{
		"connector": "GitHub",
		"provider":  "ignored",
		"priority":  "High",
		"category":  "Pull Request",
	}}

	tags := messageTags(message)
	for _, expected := range []string{"Dispatch", "platform-github", "priority-high", "category-pull-request"} {
		if !slices.Contains(tags, expected) {
			t.Fatalf("messageTags() = %v, missing %q", tags, expected)
		}
	}
}

func TestMessageTagsFallsBackToProvider(t *testing.T) {
	tags := messageTags(delivery.Message{Metadata: map[string]any{"provider": "Telegram"}})
	if !slices.Contains(tags, "platform-telegram") {
		t.Fatalf("messageTags() = %v, expected provider fallback", tags)
	}
}

func TestHeaderValueRemovesLineBreaks(t *testing.T) {
	if got := headerValue("safe\r\nX-Injected: true"); got != "safe  X-Injected: true" {
		t.Fatalf("headerValue() = %q", got)
	}
}
