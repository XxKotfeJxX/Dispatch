package routing

import (
	"time"

	"dispatch/internal/ai"
	"dispatch/internal/notification"
	"dispatch/internal/recipient"
)

type Rule struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Priority  int            `json:"priority"`
	Condition map[string]any `json:"condition"`
	Action    map[string]any `json:"action"`
	Enabled   bool           `json:"enabled"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type FinalDecision struct {
	Category       string    `json:"category"`
	Priority       string    `json:"priority"`
	Summary        string    `json:"summary"`
	Channels       []string  `json:"channels"`
	SendAt         time.Time `json:"send_at"`
	UsedAI         bool      `json:"used_ai"`
	FallbackReason string    `json:"fallback_reason,omitempty"`
	MatchedRuleID  string    `json:"matched_rule_id,omitempty"`
}

type PolicyInput struct {
	Notification notification.Notification
	Recipient    recipient.Recipient
	Rules        []Rule
	AI           *ai.Decision
	AIConfigured bool
	Now          time.Time
}
