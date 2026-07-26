package ai

import (
	"context"
	"time"

	"dispatch/internal/notification"
)

type Status string

const (
	StatusPending       Status = "pending"
	StatusCompleted     Status = "completed"
	StatusTimedOut      Status = "timed_out"
	StatusInvalidOutput Status = "invalid_output"
	StatusProviderError Status = "provider_error"
	StatusSkipped       Status = "skipped"
	StatusFallbackUsed  Status = "fallback_used"
)

type DecisionInput struct {
	EventType string         `json:"event_type"`
	Subject   string         `json:"subject"`
	Body      string         `json:"body"`
	Metadata  map[string]any `json:"metadata"`
}

type Decision struct {
	Category    string   `json:"category"`
	Priority    string   `json:"priority"`
	Summary     string   `json:"summary"`
	Confidence  float64  `json:"confidence"`
	ReasonCodes []string `json:"reason_codes"`
}

type Record struct {
	ID             string         `json:"id"`
	NotificationID string         `json:"notification_id,omitempty"`
	Provider       string         `json:"provider"`
	Model          string         `json:"model"`
	PromptVersion  string         `json:"prompt_version"`
	Decision       Decision       `json:"decision"`
	RawResponse    map[string]any `json:"raw_response"`
	DurationMS     int64          `json:"duration_ms"`
	Status         Status         `json:"status"`
	FallbackReason string         `json:"fallback_reason,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

type DecisionProvider interface {
	Decide(ctx context.Context, input DecisionInput) (Decision, map[string]any, error)
	Name() string
	Model() string
}

func InputFromNotification(item notification.Notification) DecisionInput {
	return DecisionInput{
		EventType: item.EventType, Subject: item.Subject, Body: item.Body,
		Metadata: RedactMetadata(item.Metadata),
	}
}

func RedactMetadata(metadata map[string]any) map[string]any {
	allowed := map[string]bool{
		"order_id": true, "attempt": true, "service": true, "environment": true,
		"severity": true, "region": true, "report_type": true, "error_code": true,
		"connector": true, "provider": true, "sender": true, "author_username": true,
		"sender_username": true, "chat_title": true, "channel_name": true,
		"guild_name": true, "repository": true, "action": true, "attachment_names": true,
		"actor": true, "event_title": true, "task_title": true, "channel_title": true,
	}
	result := make(map[string]any)
	for key, value := range metadata {
		if allowed[key] {
			result[key] = value
		}
	}
	return result
}

func ValidateDecision(decision Decision) bool {
	priorities := map[string]bool{"low": true, "normal": true, "high": true, "critical": true}
	if !priorities[decision.Priority] || decision.Category == "" || decision.Summary == "" ||
		decision.Confidence < 0 || decision.Confidence > 1 {
		return false
	}
	return true
}
