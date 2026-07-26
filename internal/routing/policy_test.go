package routing

import (
	"testing"
	"time"

	"dispatch/internal/ai"
	"dispatch/internal/notification"
	"dispatch/internal/recipient"
)

func TestDecideUsesRecipientChannelsAndAIOnlyForAnalysis(t *testing.T) {
	result := Decide(PolicyInput{
		Notification: notification.Notification{
			Subject:           "Invoice",
			RequestedChannels: []string{"webhook"},
		},
		Recipient: recipient.Recipient{
			Email: "a@example.test",
			Preferences: recipient.Preferences{
				DefaultChannels: []string{"email"},
			},
		},
		AI: &ai.Decision{
			Category: "finance", Priority: "high", Summary: "Invoice is ready",
			Confidence: .99, ReasonCodes: []string{"action_required"},
		},
		AIConfigured: true,
		Now:          time.Now().UTC(),
	}, .75, map[string]bool{"email": true, "webhook": true})

	if len(result.Channels) != 1 || result.Channels[0] != "email" {
		t.Fatalf("recipient destination must be definitive: %#v", result)
	}
	if !result.UsedAI || result.Category != "finance" || result.Priority != "high" {
		t.Fatalf("AI analysis was not applied: %#v", result)
	}
}

func TestDecideFallsBackFromLowConfidenceWithoutChangingDestination(t *testing.T) {
	result := Decide(PolicyInput{
		Notification: notification.Notification{Subject: "Status"},
		Recipient: recipient.Recipient{
			TelegramChatID: "1",
			Preferences: recipient.Preferences{
				DefaultChannels: []string{"telegram"},
			},
		},
		AI: &ai.Decision{
			Category: "system", Priority: "critical", Summary: "AI", Confidence: .2,
			ReasonCodes: []string{"service_disruption"},
		},
		AIConfigured: true,
		Now:          time.Now().UTC(),
	}, .75, map[string]bool{"telegram": true})

	if len(result.Channels) != 1 || result.Channels[0] != "telegram" {
		t.Fatalf("unexpected destination: %#v", result)
	}
	if result.UsedAI || result.Category != "general" ||
		result.FallbackReason != "ai_confidence_below_threshold" {
		t.Fatalf("unexpected analysis fallback: %#v", result)
	}
}

func TestDecideDoesNotRerouteUnavailableRecipientDestination(t *testing.T) {
	result := Decide(PolicyInput{
		Notification: notification.Notification{Subject: "Status"},
		Recipient: recipient.Recipient{
			Email:      "a@example.test",
			WebhookURL: "https://example.test/hook",
			Preferences: recipient.Preferences{
				DefaultChannels: []string{"email"},
			},
		},
		Now: time.Now().UTC(),
	}, .75, map[string]bool{"email": false, "webhook": true})

	if len(result.Channels) != 0 {
		t.Fatalf("unavailable destination must not silently reroute: %#v", result)
	}
}
