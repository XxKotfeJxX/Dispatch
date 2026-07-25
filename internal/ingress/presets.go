package ingress

import "strings"

var providers = map[string]struct {
	AuthMode        AuthMode
	AuthHeader      string
	SignatureHeader string
	Mapping         Mapping
}{
	"generic": {
		AuthMode: AuthBearer,
		Mapping:  Mapping{IDPath: "id", EventTypePath: "event_type", SubjectPath: "subject", BodyPath: "body", DefaultEventType: "webhook.received", DefaultSubject: "Incoming webhook"},
	},
	"github": {
		AuthMode: AuthHMAC, SignatureHeader: "X-Hub-Signature-256",
		Mapping: Mapping{IDPath: "hook.id", EventTypePath: "action", SubjectPath: "repository.full_name", BodyPath: "head_commit.message", DefaultEventType: "github.event", DefaultSubject: "GitHub event"},
	},
	"gitlab": {
		AuthMode: AuthHeader, AuthHeader: "X-Gitlab-Token",
		Mapping: Mapping{IDPath: "object_attributes.id", EventTypePath: "object_kind", SubjectPath: "project.name", BodyPath: "object_attributes.title", DefaultEventType: "gitlab.event", DefaultSubject: "GitLab event"},
	},
	"discord": {
		AuthMode: AuthBearer,
		Mapping:  Mapping{IDPath: "id", EventTypePath: "type", SubjectPath: "author.username", BodyPath: "content", DefaultEventType: "discord.message", DefaultSubject: "Discord message"},
	},
	"slack": {
		AuthMode: AuthSlack, SignatureHeader: "X-Slack-Signature",
		Mapping: Mapping{IDPath: "event_id", EventTypePath: "event.type", SubjectPath: "team_id", BodyPath: "event.text", DefaultEventType: "slack.event", DefaultSubject: "Slack event"},
	},
	"stripe": {
		AuthMode: AuthStripe, SignatureHeader: "Stripe-Signature",
		Mapping: Mapping{IDPath: "id", EventTypePath: "type", SubjectPath: "data.object.description", BodyPath: "data.object.status", DefaultEventType: "stripe.event", DefaultSubject: "Stripe event"},
	},
	"sentry": {
		AuthMode: AuthBearer,
		Mapping:  Mapping{IDPath: "data.issue.id", EventTypePath: "action", SubjectPath: "data.issue.title", BodyPath: "data.issue.culprit", DefaultEventType: "sentry.issue", DefaultSubject: "Sentry issue"},
	},
	"grafana": {
		AuthMode: AuthBearer,
		Mapping:  Mapping{IDPath: "fingerprint", EventTypePath: "status", SubjectPath: "title", BodyPath: "message", DefaultEventType: "grafana.alert", DefaultSubject: "Grafana alert"},
	},
}

func ApplyPreset(input *CreateInput) {
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	preset, ok := providers[input.Provider]
	if !ok {
		input.Provider = "generic"
		preset = providers["generic"]
	}
	if input.AuthMode == "" {
		input.AuthMode = preset.AuthMode
	}
	if input.AuthHeader == "" {
		input.AuthHeader = preset.AuthHeader
	}
	if input.Mapping.IDPath == "" {
		input.Mapping.IDPath = preset.Mapping.IDPath
	}
	if input.Mapping.EventTypePath == "" {
		input.Mapping.EventTypePath = preset.Mapping.EventTypePath
	}
	if input.Mapping.SubjectPath == "" {
		input.Mapping.SubjectPath = preset.Mapping.SubjectPath
	}
	if input.Mapping.BodyPath == "" {
		input.Mapping.BodyPath = preset.Mapping.BodyPath
	}
	if input.Mapping.DefaultEventType == "" {
		input.Mapping.DefaultEventType = preset.Mapping.DefaultEventType
	}
	if input.Mapping.DefaultSubject == "" {
		input.Mapping.DefaultSubject = preset.Mapping.DefaultSubject
	}
}

func SignatureHeader(source Source) string {
	if source.SignatureHeader != "" {
		return source.SignatureHeader
	}
	if preset, ok := providers[source.Provider]; ok {
		return preset.SignatureHeader
	}
	return "X-Dispatch-Signature"
}

func Providers() []string {
	return []string{"generic", "github", "gitlab", "discord", "slack", "stripe", "sentry", "grafana"}
}
