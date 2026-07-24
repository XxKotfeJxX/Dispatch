package routing

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"dispatch/internal/ai"
)

var channelOrder = []string{"email", "telegram", "webhook"}

func Decide(input PolicyInput, minConfidence float64, configured map[string]bool, fallbackWebhook bool) FinalDecision {
	result := FinalDecision{
		Category: "general", Priority: "normal", Summary: input.Notification.Subject,
		SendAt: input.Now,
	}
	if input.Notification.ScheduledAt != nil && input.Notification.ScheduledAt.After(input.Now) {
		result.SendAt = *input.Notification.ScheduledAt
	}

	hardChannels, matchedRule := matchingRule(input)
	if len(input.Notification.RequestedChannels) > 0 {
		result.Channels = append([]string{}, input.Notification.RequestedChannels...)
		result.FallbackReason = "explicit_requested_channels"
	} else if len(hardChannels) > 0 {
		result.Channels, result.MatchedRuleID = hardChannels, matchedRule
		result.FallbackReason = "deterministic_rule"
	} else if input.AI != nil && ai.ValidateDecision(*input.AI) && input.AI.Confidence >= minConfidence {
		result.Category, result.Priority, result.Summary = input.AI.Category, input.AI.Priority, input.AI.Summary
		result.Channels = append([]string{}, input.AI.RecommendedChannels...)
		result.UsedAI = true
	} else {
		result.Channels = append([]string{}, input.Recipient.Preferences.DefaultChannels...)
		if input.AI != nil && input.AI.Confidence < minConfidence {
			result.FallbackReason = "ai_confidence_below_threshold"
		} else if !input.AIConfigured {
			result.FallbackReason = "ai_disabled_or_unconfigured"
		} else {
			result.FallbackReason = "ai_unavailable"
		}
	}

	disabled := make(map[string]bool)
	for _, channel := range input.Recipient.Preferences.DisabledChannels {
		disabled[channel] = true
	}
	available := destinationAvailability(input)
	filtered := make([]string, 0, 3)
	for _, candidate := range channelOrder {
		if slices.Contains(result.Channels, candidate) && configured[candidate] && available[candidate] && !disabled[candidate] {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 0 {
		for _, candidate := range input.Recipient.Preferences.DefaultChannels {
			if configured[candidate] && available[candidate] && !disabled[candidate] {
				filtered = append(filtered, candidate)
				break
			}
		}
	}
	if len(filtered) == 0 && fallbackWebhook && configured["webhook"] && available["webhook"] && !disabled["webhook"] {
		filtered = append(filtered, "webhook")
	}
	if len(filtered) > 3 {
		filtered = filtered[:3]
	}
	result.Channels = filtered
	if quietUntil, ok := quietHoursEnd(input.Recipient.Preferences.QuietHoursStart,
		input.Recipient.Preferences.QuietHoursEnd, input.Recipient.Preferences.TimeZone, result.SendAt); ok {
		result.SendAt = quietUntil
	}
	return result
}

func matchingRule(input PolicyInput) ([]string, string) {
	for _, rule := range input.Rules {
		if !rule.Enabled {
			continue
		}
		eventType, _ := rule.Condition["event_type"].(string)
		priority, _ := rule.Condition["priority"].(string)
		if eventType != "" && eventType != input.Notification.EventType {
			continue
		}
		if priority != "" && priority != input.Notification.Priority {
			continue
		}
		raw, ok := rule.Action["channels"].([]any)
		if !ok {
			if typed, valid := rule.Action["channels"].([]string); valid {
				return typed, rule.ID
			}
			continue
		}
		channels := make([]string, 0, len(raw))
		for _, item := range raw {
			channels = append(channels, fmt.Sprint(item))
		}
		return channels, rule.ID
	}
	return nil, ""
}

func destinationAvailability(input PolicyInput) map[string]bool {
	return map[string]bool{
		"email":    input.Recipient.Email != "",
		"telegram": input.Recipient.TelegramChatID != "",
		"webhook":  input.Recipient.WebhookURL != "",
	}
}

func quietHoursEnd(start, end, zone string, now time.Time) (time.Time, bool) {
	if start == "" || end == "" {
		return time.Time{}, false
	}
	location := time.UTC
	if zone != "" {
		if parsed, err := time.LoadLocation(zone); err == nil {
			location = parsed
		}
	}
	local := now.In(location)
	parseClock := func(value string) (int, int, bool) {
		parsed, err := time.Parse("15:04", strings.TrimSpace(value))
		return parsed.Hour(), parsed.Minute(), err == nil
	}
	startHour, startMinute, startOK := parseClock(start)
	endHour, endMinute, endOK := parseClock(end)
	if !startOK || !endOK {
		return time.Time{}, false
	}
	startAt := time.Date(local.Year(), local.Month(), local.Day(), startHour, startMinute, 0, 0, location)
	endAt := time.Date(local.Year(), local.Month(), local.Day(), endHour, endMinute, 0, 0, location)
	if !endAt.After(startAt) {
		if local.Before(endAt) {
			startAt = startAt.AddDate(0, 0, -1)
		} else {
			endAt = endAt.AddDate(0, 0, 1)
		}
	}
	if !local.Before(startAt) && local.Before(endAt) {
		return endAt.UTC(), true
	}
	return time.Time{}, false
}
