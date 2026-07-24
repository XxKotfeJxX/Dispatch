package delivery

import (
	"context"
	"fmt"
	"time"
)

type Channel string

const (
	ChannelEmail    Channel = "email"
	ChannelTelegram Channel = "telegram"
	ChannelWebhook  Channel = "webhook"
)

type Status string

const (
	StatusQueued     Status = "queued"
	StatusSending    Status = "sending"
	StatusRetryWait  Status = "retry_wait"
	StatusDelivered  Status = "delivered"
	StatusFailed     Status = "failed"
	StatusDeadLetter Status = "dead_letter"
	StatusCancelled  Status = "cancelled"
)

type Delivery struct {
	ID               string    `json:"id"`
	NotificationID   string    `json:"notification_id"`
	Channel          Channel   `json:"channel"`
	Destination      string    `json:"destination"`
	Status           Status    `json:"status"`
	AttemptCount     int       `json:"attempt_count"`
	NextAttemptAt    time.Time `json:"next_attempt_at"`
	LastErrorCode    string    `json:"last_error_code,omitempty"`
	LastErrorMessage string    `json:"last_error_message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Attempt struct {
	ID            string     `json:"id"`
	DeliveryID    string     `json:"delivery_id"`
	AttemptNumber int        `json:"attempt_number"`
	Provider      string     `json:"provider"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Status        string     `json:"status"`
	ResponseCode  int        `json:"response_code,omitempty"`
	ErrorCode     string     `json:"error_code,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
}

type Message struct {
	DeliveryID     string
	NotificationID string
	Destination    string
	Subject        string
	Body           string
	Metadata       map[string]any
}

type ProviderResult struct {
	ResponseCode int
	ProviderID   string
}

type Provider interface {
	Channel() Channel
	Configured() bool
	Deliver(ctx context.Context, message Message) (ProviderResult, error)
}

type ProviderError struct {
	Code      string
	Message   string
	Retryable bool
}

func (err *ProviderError) Error() string {
	return fmt.Sprintf("%s: %s", err.Code, err.Message)
}
