package notification

import "time"

type Status string

const (
	StatusReceived           Status = "received"
	StatusAnalyzing          Status = "analyzing"
	StatusQueued             Status = "queued"
	StatusDelivering         Status = "delivering"
	StatusDelivered          Status = "delivered"
	StatusPartiallyDelivered Status = "partially_delivered"
	StatusFailed             Status = "failed"
	StatusCancelled          Status = "cancelled"
)

type Notification struct {
	ID                string         `json:"id"`
	IdempotencyKey    string         `json:"idempotency_key"`
	RecipientID       string         `json:"recipient_id"`
	EventType         string         `json:"event_type"`
	Subject           string         `json:"subject"`
	Body              string         `json:"body"`
	Metadata          map[string]any `json:"metadata"`
	RequestedChannels []string       `json:"requested_channels"`
	ScheduledAt       *time.Time     `json:"scheduled_at,omitempty"`
	Category          string         `json:"category,omitempty"`
	Priority          string         `json:"priority"`
	Summary           string         `json:"summary,omitempty"`
	Status            Status         `json:"status"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type CreateInput struct {
	IdempotencyKey    string         `json:"idempotency_key"`
	RecipientID       string         `json:"recipient_id"`
	EventType         string         `json:"event_type"`
	Subject           string         `json:"subject"`
	Body              string         `json:"body"`
	Metadata          map[string]any `json:"metadata"`
	RequestedChannels []string       `json:"requested_channels"`
	ScheduledAt       *time.Time     `json:"scheduled_at"`
}

type Filter struct {
	Status     string
	Priority   string
	Channel    string
	EventType  string
	AIFallback *bool
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

func CanTransition(from, to Status) bool {
	allowed := map[Status]map[Status]bool{
		StatusReceived:           {StatusAnalyzing: true, StatusQueued: true, StatusCancelled: true},
		StatusAnalyzing:          {StatusQueued: true, StatusFailed: true, StatusCancelled: true},
		StatusQueued:             {StatusDelivering: true, StatusCancelled: true},
		StatusDelivering:         {StatusDelivered: true, StatusPartiallyDelivered: true, StatusFailed: true, StatusCancelled: true},
		StatusFailed:             {StatusQueued: true, StatusCancelled: true},
		StatusPartiallyDelivered: {StatusQueued: true, StatusDelivered: true, StatusFailed: true, StatusCancelled: true},
	}
	return allowed[from][to]
}
