package ingress

import "testing"

func TestTransformUsesNestedMapping(t *testing.T) {
	source := Source{
		ID: "src_1", Slug: "monitor", Provider: "generic",
		Mapping: Mapping{
			IDPath: "event.id", EventTypePath: "event.kind",
			SubjectPath: "event.title", BodyPath: "event.details.message",
		},
	}
	result, err := Transform(source, []byte(`{"event":{"id":"42","kind":"alert","title":"CPU high","details":{"message":"95%"}}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExternalID != "42" || result.EventType != "alert" ||
		result.Subject != "CPU high" || result.Body != "95%" {
		t.Fatalf("unexpected transform: %#v", result)
	}
}

func TestGitHubHeadersOverridePayload(t *testing.T) {
	input := CreateInput{Provider: "github"}
	ApplyPreset(&input)
	source := Source{ID: "src", Slug: "github", Provider: "github", Mapping: input.Mapping}
	result, err := Transform(source, []byte(`{"action":"opened","repository":{"full_name":"owner/repo"}}`),
		map[string]string{"x-github-event": "pull_request", "x-github-delivery": "delivery-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.EventType != "github.pull_request" || result.ExternalID != "delivery-1" ||
		result.Subject != "owner/repo" {
		t.Fatalf("unexpected GitHub transform: %#v", result)
	}
}

func TestPresetPreservesRequestedChannels(t *testing.T) {
	input := CreateInput{Provider: "grafana", Mapping: Mapping{RequestedChannels: []string{"telegram"}}}
	ApplyPreset(&input)
	if len(input.Mapping.RequestedChannels) != 1 || input.Mapping.RequestedChannels[0] != "telegram" {
		t.Fatal("preset discarded requested channels")
	}
	if input.Mapping.SubjectPath != "title" {
		t.Fatal("preset was not applied")
	}
}
