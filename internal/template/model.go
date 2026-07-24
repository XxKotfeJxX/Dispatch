package template

import "time"

type Template struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Channel         string    `json:"channel"`
	SubjectTemplate string    `json:"subject_template,omitempty"`
	BodyTemplate    string    `json:"body_template"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
