package jobs

import "time"

type Job struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Payload      map[string]any `json:"payload"`
	Status       string         `json:"status"`
	AttemptCount int            `json:"attempt_count"`
	MaxAttempts  int            `json:"max_attempts"`
	AvailableAt  time.Time      `json:"available_at"`
	LockedAt     *time.Time     `json:"locked_at,omitempty"`
	LockedBy     string         `json:"locked_by,omitempty"`
	LastError    string         `json:"last_error,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

func RetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 30 * time.Second
	case 2:
		return 2 * time.Minute
	case 3:
		return 10 * time.Minute
	default:
		return 30 * time.Minute
	}
}
