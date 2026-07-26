package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

var Categories = []string{
	"security",
	"finance",
	"development",
	"communication",
	"calendar",
	"tasks",
	"files",
	"account",
	"system",
	"content",
	"general",
}

var ReasonCodes = []string{
	"security_risk",
	"financial_risk",
	"service_disruption",
	"action_required",
	"deadline_soon",
	"direct_message",
	"direct_mention",
	"status_change",
	"new_content",
	"routine_update",
	"promotional",
	"insufficient_context",
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

type ProviderFailure struct {
	Kind       string
	StatusCode int
	Err        error
}

func (failure *ProviderFailure) Error() string {
	if failure.StatusCode > 0 {
		return fmt.Sprintf("%s (HTTP %d): %v", failure.Kind, failure.StatusCode, failure.Err)
	}
	return fmt.Sprintf("%s: %v", failure.Kind, failure.Err)
}

func (failure *ProviderFailure) Unwrap() error { return failure.Err }

func FallbackReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	var failure *ProviderFailure
	if errors.As(err, &failure) {
		if failure.StatusCode > 0 {
			return fmt.Sprintf("%s_http_%d", failure.Kind, failure.StatusCode)
		}
		return failure.Kind
	}
	return "provider_error"
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
		"sender_username": true, "sender_first_name": true, "sender_last_name": true,
		"chat_title": true, "chat_type": true, "channel_name": true,
		"guild_name": true, "repository": true, "action": true, "attachment_names": true,
		"actor": true, "event_title": true, "task_title": true, "channel_title": true,
		"account_label": true, "connection_name": true, "mentioned": true,
		"forwarded": true, "pinned": true, "has_media": true, "media_type": true,
		"timestamp": true, "published_at": true, "start": true, "end": true,
		"due": true, "status": true, "completed": true, "organizer": true,
		"creator": true, "attendees": true, "list_name": true, "space_name": true,
	}
	result := make(map[string]any)
	for key, value := range metadata {
		if allowed[key] {
			result[key] = compactMetadataValue(value)
		}
	}
	return result
}

func LimitInput(input DecisionInput, maxChars int) DecisionInput {
	input.EventType = truncate(input.EventType, 200)
	input.Subject = truncate(input.Subject, 1000)
	remaining := maxChars - utf8.RuneCountInString(input.EventType) -
		utf8.RuneCountInString(input.Subject)
	if remaining < 0 {
		remaining = 0
	}
	input.Body = truncate(input.Body, remaining)
	return input
}

func compactMetadataValue(value any) any {
	switch typed := value.(type) {
	case string:
		return truncate(typed, 500)
	case []string:
		if len(typed) > 20 {
			typed = typed[:20]
		}
		result := make([]string, len(typed))
		for index, item := range typed {
			result[index] = truncate(item, 200)
		}
		return result
	case []any:
		if len(typed) > 20 {
			typed = typed[:20]
		}
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = compactMetadataValue(item)
		}
		return result
	default:
		return value
	}
}

func truncate(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	if maximum == 1 {
		return "…"
	}
	return string(runes[:maximum-1]) + "…"
}

func NormalizeDecision(decision Decision) Decision {
	decision.Category = strings.ToLower(strings.TrimSpace(decision.Category))
	decision.Priority = strings.ToLower(strings.TrimSpace(decision.Priority))
	decision.Summary = strings.Join(strings.Fields(strings.TrimSpace(decision.Summary)), " ")
	seen := map[string]bool{}
	reasons := make([]string, 0, len(decision.ReasonCodes))
	for _, reason := range decision.ReasonCodes {
		reason = strings.ToLower(strings.TrimSpace(reason))
		if reason != "" && !seen[reason] {
			seen[reason] = true
			reasons = append(reasons, reason)
		}
	}
	decision.ReasonCodes = reasons
	return decision
}

func ValidateDecision(decision Decision) bool {
	return ValidateDecisionWithSummaryLimit(decision, 500)
}

func ValidateDecisionWithSummaryLimit(decision Decision, summaryMaxChars int) bool {
	decision = NormalizeDecision(decision)
	if !contains(Categories, decision.Category) ||
		!contains([]string{"low", "normal", "high", "critical"}, decision.Priority) ||
		decision.Summary == "" || utf8.RuneCountInString(decision.Summary) > summaryMaxChars ||
		decision.Confidence < 0 || decision.Confidence > 1 {
		return false
	}
	if len(decision.ReasonCodes) < 1 || len(decision.ReasonCodes) > 5 {
		return false
	}
	for _, reason := range decision.ReasonCodes {
		if !contains(ReasonCodes, reason) {
			return false
		}
	}
	return true
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
