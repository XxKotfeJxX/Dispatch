package routing

import (
	"time"

	"dispatch/internal/ai"
	"dispatch/internal/notification"
	"dispatch/internal/recipient"
)

type FinalDecision struct {
	Category       string    `json:"category"`
	Priority       string    `json:"priority"`
	Summary        string    `json:"summary"`
	Channels       []string  `json:"channels"`
	SendAt         time.Time `json:"send_at"`
	UsedAI         bool      `json:"used_ai"`
	FallbackReason string    `json:"fallback_reason,omitempty"`
}

type PolicyInput struct {
	Notification notification.Notification
	Recipient    recipient.Recipient
	AI           *ai.Decision
	AIConfigured bool
	Now          time.Time
}
