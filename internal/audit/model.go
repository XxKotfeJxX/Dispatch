package audit

import "time"

type Event struct {
	ID         int64          `json:"id"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	EventType  string         `json:"event_type"`
	Details    map[string]any `json:"details"`
	CreatedAt  time.Time      `json:"created_at"`
}
