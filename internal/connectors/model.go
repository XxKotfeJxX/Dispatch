package connectors

import "time"

type AuthKind string
type Transport string
type Availability string

const (
	AuthNone       AuthKind = "none"
	AuthBotToken   AuthKind = "bot_token"
	AuthOAuth2     AuthKind = "oauth2"
	AuthAppInstall AuthKind = "app_install"
	AuthAPIKey     AuthKind = "api_key"

	TransportInternal Transport = "internal"
	TransportWebhook  Transport = "webhook"
	TransportGateway  Transport = "gateway"
	TransportPolling  Transport = "polling"
	TransportWebSub   Transport = "websub"

	Available     Availability = "available"
	SetupRequired Availability = "setup_required"
	Limited       Availability = "limited"
	Unavailable   Availability = "unavailable"
)

type Field struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
}

type Manifest struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Summary          string       `json:"summary"`
	Category         string       `json:"category"`
	Auth             AuthKind     `json:"auth"`
	Transport        Transport    `json:"transport"`
	Availability     Availability `json:"availability"`
	Configured       bool         `json:"configured"`
	Capabilities     []string     `json:"capabilities"`
	Fields           []Field      `json:"fields"`
	SetupHint        string       `json:"setup_hint,omitempty"`
	Documentation    string       `json:"documentation,omitempty"`
	AuthorizationURL string       `json:"authorization_url,omitempty"`
}

type Connection struct {
	ID             string            `json:"id"`
	ConnectorID    string            `json:"connector_id"`
	Name           string            `json:"name"`
	RecipientID    string            `json:"recipient_id"`
	Status         string            `json:"status"`
	AccountLabel   string            `json:"account_label,omitempty"`
	Config         map[string]string `json:"config"`
	TokenExpiresAt *time.Time        `json:"token_expires_at,omitempty"`
	LastTestedAt   *time.Time        `json:"last_tested_at,omitempty"`
	LastEventAt    *time.Time        `json:"last_event_at,omitempty"`
	LastError      string            `json:"last_error,omitempty"`
	Enabled        bool              `json:"enabled"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type CreateInput struct {
	ConnectorID string            `json:"connector_id"`
	Name        string            `json:"name"`
	RecipientID string            `json:"recipient_id"`
	Config      map[string]string `json:"config"`
	Credentials map[string]string `json:"credentials"`
}

type OAuthStartInput struct {
	Name        string `json:"name"`
	RecipientID string `json:"recipient_id"`
}

type OAuthState struct {
	StateHash      []byte
	ConnectorID    string
	ConnectionName string
	RecipientID    string
	VerifierCipher []byte
	ExpiresAt      time.Time
}

type Credentials struct {
	Values       map[string]string `json:"values"`
	AccessToken  string            `json:"access_token,omitempty"`
	RefreshToken string            `json:"refresh_token,omitempty"`
	TokenType    string            `json:"token_type,omitempty"`
	Scope        string            `json:"scope,omitempty"`
	ExpiresAt    *time.Time        `json:"expires_at,omitempty"`
}

type TestResult struct {
	OK           bool   `json:"ok"`
	AccountLabel string `json:"account_label,omitempty"`
	Message      string `json:"message"`
}

type NormalizedEvent struct {
	ExternalID string
	EventType  string
	Subject    string
	Body       string
	Metadata   map[string]any
}
