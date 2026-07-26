package template

import (
	"testing"
	"time"

	"dispatch/internal/notification"
)

func TestSelectPrefersSpecificConditionalTemplate(t *testing.T) {
	now := time.Now()
	items := []Template{
		{Name: "global", Service: ServiceAny, Channel: ChannelAll, Enabled: true, BodyTemplate: "{{body}}", UpdatedAt: now},
		{Name: "github fallback", Service: "github", Channel: ChannelAll, Enabled: true, BodyTemplate: "{{body}}", UpdatedAt: now},
		{Name: "github sender", Service: "github", Channel: ChannelAll, Enabled: true, BodyTemplate: "{{sender}}: {{body}}",
			Conditions: []Condition{{Field: "sender", Operator: "equals", Value: "octocat"}}, UpdatedAt: now},
	}
	item := notification.Notification{
		EventType: "github.issues", Body: "Issue opened",
		Metadata: map[string]any{"connector": "github", "sender": "octocat"},
	}
	selected := Select(items, item, "email")
	if selected == nil || selected.Name != "github sender" {
		t.Fatalf("selected %#v", selected)
	}
}

func TestRenderUsesCommonAndMetadataVariables(t *testing.T) {
	item := notification.Notification{
		EventType: "github.pull_request", Subject: "PR opened", Body: "Add templates",
		Metadata: map[string]any{"connector": "github", "sender": "octocat", "repository": "owner/repo"},
	}
	subject, body := Render(Template{
		SubjectTemplate: "[{{service}}] {{subject}}",
		BodyTemplate:    "{{sender}} changed {{repository}}\n{{body}}\n{{missing}}",
	}, item)
	if subject != "[github] PR opened" {
		t.Fatalf("subject = %q", subject)
	}
	if body != "octocat changed owner/repo\nAdd templates\n" {
		t.Fatalf("body = %q", body)
	}
}
