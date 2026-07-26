package recipient

import "time"

type Preferences struct {
	DefaultChannels  []string `json:"default_channels"`
	DisabledChannels []string `json:"disabled_channels"`
	QuietHoursStart  string   `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd    string   `json:"quiet_hours_end,omitempty"`
	TimeZone         string   `json:"time_zone,omitempty"`
}

type Recipient struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	DestinationType  string      `json:"destination_type"`
	DestinationLabel string      `json:"destination_label,omitempty"`
	Email            string      `json:"email,omitempty"`
	TelegramChatID   string      `json:"telegram_chat_id,omitempty"`
	WebhookURL       string      `json:"webhook_url,omitempty"`
	Preferences      Preferences `json:"preferences"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

type Setup struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	Target      string    `json:"target,omitempty"`
	Status      string    `json:"status"`
	RecipientID string    `json:"recipient_id,omitempty"`
	CodeHash    []byte    `json:"-"`
	Attempts    int       `json:"-"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
