package routing

import (
	"testing"
	"time"

	"dispatch/internal/ai"
	"dispatch/internal/notification"
	"dispatch/internal/recipient"
)

func TestDecidePrecedence(t *testing.T) {
	now := time.Now().UTC()
	base := PolicyInput{
		Notification: notification.Notification{EventType: "invoice.ready", Subject: "Invoice", RequestedChannels: []string{"email"}},
		Recipient:    recipient.Recipient{Email: "a@example.test", TelegramChatID: "1", Preferences: recipient.Preferences{DefaultChannels: []string{"telegram"}}},
		Rules:        []Rule{{ID: "rule", Enabled: true, Condition: map[string]any{"event_type": "invoice.ready"}, Action: map[string]any{"channels": []string{"telegram"}}}},
		AI:           &ai.Decision{Category: "billing", Priority: "high", Summary: "AI", RecommendedChannels: []string{"telegram"}, Confidence: .99},
		AIConfigured: true, Now: now,
	}
	result := Decide(base, .75, map[string]bool{"email": true, "telegram": true}, false)
	if len(result.Channels) != 1 || result.Channels[0] != "email" || result.UsedAI {
		t.Fatalf("explicit channel must win: %#v", result)
	}
	base.Notification.RequestedChannels = nil
	result = Decide(base, .75, map[string]bool{"email": true, "telegram": true}, false)
	if result.Channels[0] != "telegram" || result.MatchedRuleID != "rule" {
		t.Fatalf("rule must win over AI: %#v", result)
	}
}

func TestDecideFallsBackOnLowConfidenceAndDisabledChannel(t *testing.T) {
	result := Decide(PolicyInput{
		Notification: notification.Notification{Subject: "Status"},
		Recipient: recipient.Recipient{
			Email: "a@example.test", TelegramChatID: "1",
			Preferences: recipient.Preferences{DefaultChannels: []string{"email"}, DisabledChannels: []string{"telegram"}},
		},
		AI:           &ai.Decision{Category: "ops", Priority: "normal", Summary: "AI", RecommendedChannels: []string{"telegram"}, Confidence: .2},
		AIConfigured: true, Now: time.Now(),
	}, .75, map[string]bool{"email": true, "telegram": true}, false)
	if len(result.Channels) != 1 || result.Channels[0] != "email" || result.FallbackReason != "ai_confidence_below_threshold" {
		t.Fatalf("unexpected fallback: %#v", result)
	}
}
