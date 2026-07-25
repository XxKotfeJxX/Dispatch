package ingress

import "time"

type AuthMode string

const (
	AuthBearer AuthMode = "bearer"
	AuthHeader AuthMode = "header"
	AuthHMAC   AuthMode = "hmac_sha256"
	AuthSlack  AuthMode = "slack_signature"
	AuthStripe AuthMode = "stripe_signature"
)

type Mapping struct {
	IDPath            string   `json:"id_path,omitempty"`
	EventTypePath     string   `json:"event_type_path,omitempty"`
	SubjectPath       string   `json:"subject_path,omitempty"`
	BodyPath          string   `json:"body_path,omitempty"`
	DefaultEventType  string   `json:"default_event_type"`
	DefaultSubject    string   `json:"default_subject"`
	RequestedChannels []string `json:"requested_channels"`
}

type Source struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	Provider        string    `json:"provider"`
	RecipientID     string    `json:"recipient_id"`
	AuthMode        AuthMode  `json:"auth_mode"`
	AuthHeader      string    `json:"auth_header,omitempty"`
	SignatureHeader string    `json:"signature_header,omitempty"`
	Mapping         Mapping   `json:"mapping"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Provider    string   `json:"provider"`
	RecipientID string   `json:"recipient_id"`
	AuthMode    AuthMode `json:"auth_mode,omitempty"`
	AuthHeader  string   `json:"auth_header,omitempty"`
	Secret      string   `json:"secret,omitempty"`
	Mapping     Mapping  `json:"mapping"`
}

type Created struct {
	Source Source `json:"source"`
	Secret string `json:"secret"`
}

type Event struct {
	ID             int64     `json:"id"`
	SourceID       string    `json:"source_id"`
	ExternalID     string    `json:"external_id"`
	NotificationID string    `json:"notification_id"`
	EventType      string    `json:"event_type"`
	PayloadDigest  string    `json:"payload_digest"`
	ReceivedAt     time.Time `json:"received_at"`
}
