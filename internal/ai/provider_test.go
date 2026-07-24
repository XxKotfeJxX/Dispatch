package ai

import "testing"

func TestRedactMetadataUsesAllowList(t *testing.T) {
	result := RedactMetadata(map[string]any{"order_id": "42", "password": "secret", "token": "secret"})
	if result["order_id"] != "42" || result["password"] != nil || result["token"] != nil {
		t.Fatalf("redaction failed: %#v", result)
	}
}

func TestValidateDecisionRejectsUnknownChannel(t *testing.T) {
	if ValidateDecision(Decision{Category: "ops", Priority: "normal", Summary: "x", RecommendedChannels: []string{"sms"}, Confidence: .9}) {
		t.Fatal("unknown channel accepted")
	}
}
