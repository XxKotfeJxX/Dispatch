package template

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"dispatch/internal/notification"
)

const (
	ServiceAny = "any"
	ChannelAll = "all"
)

type Condition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type Template struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Channel         string      `json:"channel"`
	Service         string      `json:"service"`
	Conditions      []Condition `json:"conditions"`
	Enabled         bool        `json:"enabled"`
	SubjectTemplate string      `json:"subject_template,omitempty"`
	BodyTemplate    string      `json:"body_template"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

func Service(item notification.Notification) string {
	if value := stringValue(item.Metadata["connector"]); value != "" {
		return strings.ToLower(value)
	}
	if value := stringValue(item.Metadata["source"]); value != "" {
		return strings.ToLower(value)
	}
	if index := strings.IndexByte(item.EventType, '.'); index > 0 {
		return strings.ToLower(item.EventType[:index])
	}
	return ServiceAny
}

func Select(items []Template, item notification.Notification, channel string) *Template {
	service := Service(item)
	sort.SliceStable(items, func(i, j int) bool {
		left, right := score(items[i], service, channel), score(items[j], service, channel)
		if left != right {
			return left > right
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	for index := range items {
		candidate := &items[index]
		if score(*candidate, service, channel) < 0 || !matches(*candidate, item) {
			continue
		}
		return candidate
	}
	return nil
}

func Render(item Template, notificationItem notification.Notification) (string, string) {
	values := Values(notificationItem)
	subject := notificationItem.Subject
	if strings.TrimSpace(item.SubjectTemplate) != "" {
		subject = render(item.SubjectTemplate, values)
	}
	return subject, render(item.BodyTemplate, values)
}

func Values(item notification.Notification) map[string]string {
	values := map[string]string{
		"subject": item.Subject, "body": item.Body, "content": item.Subject + "\n" + item.Body,
		"event_type": item.EventType, "service": Service(item), "category": item.Category,
		"priority": item.Priority, "summary": item.Summary,
	}
	for key, value := range item.Metadata {
		rendered := stringValue(value)
		values["metadata."+key] = rendered
		if _, exists := values[key]; !exists {
			values[key] = rendered
		}
	}
	values["sender"] = first(values, "sender", "from", "author_username", "actor")
	values["provider"] = first(values, "provider", "service")
	return values
}

func Validate(item Template) error {
	if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.BodyTemplate) == "" {
		return fmt.Errorf("name and message body are required")
	}
	if item.Service == "" {
		item.Service = ServiceAny
	}
	if item.Channel == "" {
		item.Channel = ChannelAll
	}
	services := map[string]bool{
		ServiceAny: true, "github": true, "google": true, "discord": true,
		"telegram": true, "youtube": true, "webhook": true,
	}
	if !services[item.Service] {
		return fmt.Errorf("unsupported service")
	}
	if item.Channel != ChannelAll && item.Channel != "email" &&
		item.Channel != "telegram" && item.Channel != "webhook" {
		return fmt.Errorf("unsupported delivery channel")
	}
	for _, condition := range item.Conditions {
		if strings.TrimSpace(condition.Field) == "" || strings.TrimSpace(condition.Value) == "" {
			return fmt.Errorf("condition field and value are required")
		}
		switch condition.Operator {
		case "contains", "equals", "starts_with", "not_contains":
		default:
			return fmt.Errorf("unsupported condition operator")
		}
	}
	if len(item.Name) > 120 || len(item.SubjectTemplate) > 2_000 ||
		len(item.BodyTemplate) > 20_000 || len(item.Conditions) > 10 {
		return fmt.Errorf("template exceeds the allowed size")
	}
	return nil
}

func score(item Template, service, channel string) int {
	if !item.Enabled || (item.Service != ServiceAny && item.Service != service) ||
		(item.Channel != ChannelAll && item.Channel != channel) {
		return -1
	}
	result := len(item.Conditions) * 100
	if item.Service == service {
		result += 10_000
	}
	if item.Channel == channel {
		result += 1_000
	}
	return result
}

func matches(item Template, notificationItem notification.Notification) bool {
	values := Values(notificationItem)
	for _, condition := range item.Conditions {
		actual := strings.ToLower(strings.TrimSpace(values[condition.Field]))
		expected := strings.ToLower(strings.TrimSpace(condition.Value))
		matched := false
		switch condition.Operator {
		case "contains":
			matched = strings.Contains(actual, expected)
		case "not_contains":
			matched = !strings.Contains(actual, expected)
		case "equals":
			matched = actual == expected
		case "starts_with":
			matched = strings.HasPrefix(actual, expected)
		}
		if !matched {
			return false
		}
	}
	return true
}

func render(value string, values map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := placeholderPattern.FindStringSubmatch(match)
		return values[parts[1]]
	})
}

func first(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if values[key] != "" {
			return values[key]
		}
	}
	return ""
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(raw)
	}
}
