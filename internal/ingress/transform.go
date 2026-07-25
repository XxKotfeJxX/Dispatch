package ingress

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Transformed struct {
	ExternalID string
	EventType  string
	Subject    string
	Body       string
	Metadata   map[string]any
}

func Transform(source Source, raw []byte, headers map[string]string) (Transformed, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Transformed{}, fmt.Errorf("payload must be a JSON object: %w", err)
	}
	result := Transformed{
		ExternalID: stringAt(payload, source.Mapping.IDPath),
		EventType:  stringAt(payload, source.Mapping.EventTypePath),
		Subject:    stringAt(payload, source.Mapping.SubjectPath),
		Body:       stringAt(payload, source.Mapping.BodyPath),
		Metadata: map[string]any{
			"source": source.Provider, "source_id": source.ID, "source_slug": source.Slug,
		},
	}
	if source.Provider == "github" {
		if value := headers["x-github-event"]; value != "" {
			result.EventType = "github." + value
		}
		if value := headers["x-github-delivery"]; value != "" {
			result.ExternalID = value
		}
	}
	if source.Provider == "gitlab" {
		if value := headers["x-gitlab-event-uuid"]; value != "" {
			result.ExternalID = value
		}
	}
	if result.EventType == "" {
		result.EventType = source.Mapping.DefaultEventType
	}
	if result.Subject == "" {
		result.Subject = source.Mapping.DefaultSubject
	}
	if result.Body == "" {
		if value := firstString(payload, "message", "text", "description", "title"); value != "" {
			result.Body = value
		} else {
			compact, _ := json.Marshal(payload)
			result.Body = string(compact)
		}
	}
	if len(result.Subject) > 200 {
		result.Subject = result.Subject[:200]
	}
	if len(result.Body) > 100_000 {
		result.Body = result.Body[:100_000]
	}
	return result, nil
}

func stringAt(value any, path string) string {
	if path == "" {
		return ""
	}
	current := value
	for _, component := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[component]
		case []any:
			index, err := strconv.Atoi(component)
			if err != nil || index < 0 || index >= len(typed) {
				return ""
			}
			current = typed[index]
		default:
			return ""
		}
	}
	switch typed := current.(type) {
	case string:
		return typed
	case float64, bool, json.Number:
		return fmt.Sprint(typed)
	default:
		return ""
	}
}

func firstString(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		if value := stringAt(payload, path); value != "" {
			return value
		}
	}
	return ""
}
