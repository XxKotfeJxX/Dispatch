package ai

import (
	"strings"
	"testing"
	"unicode/utf8"

	"dispatch/internal/notification"
)

func TestValidateDecisionAcceptsAnalysis(t *testing.T) {
	if !ValidateDecision(Decision{
		Category: "system", Priority: "normal", Summary: "Service recovered", Confidence: .9,
		ReasonCodes: []string{"status_change"},
	}) {
		t.Fatal("expected a valid analysis decision")
	}
}

func TestValidateDecisionRejectsInvalidPriority(t *testing.T) {
	if ValidateDecision(Decision{
		Category: "system", Priority: "urgent", Summary: "Service recovered", Confidence: .9,
		ReasonCodes: []string{"status_change"},
	}) {
		t.Fatal("expected invalid priority to be rejected")
	}
}

func TestValidateDecisionRejectsInvalidConfidence(t *testing.T) {
	if ValidateDecision(Decision{
		Category: "system", Priority: "normal", Summary: "Service recovered", Confidence: 1.1,
		ReasonCodes: []string{"status_change"},
	}) {
		t.Fatal("expected invalid confidence to be rejected")
	}
}

func TestValidateDecisionRejectsUnknownCategoryAndReason(t *testing.T) {
	if ValidateDecision(Decision{
		Category: "whatever", Priority: "normal", Summary: "Service recovered", Confidence: .9,
		ReasonCodes: []string{"made_up"},
	}) {
		t.Fatal("expected unknown taxonomy values to be rejected")
	}
}

func TestInputFromNotificationKeepsUsefulMetadataAndBoundsValues(t *testing.T) {
	input := InputFromNotification(notification.Notification{Metadata: map[string]any{
		"sender_username":  strings.Repeat("a", 700),
		"attachment_names": []any{"report.xlsx", "brief.pdf"},
		"secret":           "must not leave Dispatch",
	}})
	if _, ok := input.Metadata["secret"]; ok {
		t.Fatal("sensitive unlisted metadata must be removed")
	}
	if utf8.RuneCountInString(input.Metadata["sender_username"].(string)) != 500 {
		t.Fatal("metadata strings must be bounded")
	}
	if len(input.Metadata["attachment_names"].([]any)) != 2 {
		t.Fatal("safe attachment names should be preserved")
	}
}

func TestLimitInputTruncatesLargeNotificationBody(t *testing.T) {
	input := LimitInput(DecisionInput{
		EventType: "gmail.message", Subject: "Report", Body: strings.Repeat("x", 5000),
	}, 1000)
	if utf8.RuneCountInString(input.EventType)+utf8.RuneCountInString(input.Subject)+
		utf8.RuneCountInString(input.Body) > 1000 {
		t.Fatal("AI input exceeded configured limit")
	}
}
