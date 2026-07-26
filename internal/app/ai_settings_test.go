package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dispatch/internal/ai"
	"dispatch/internal/config"
)

type captureAIProvider struct {
	input ai.DecisionInput
}

func (provider *captureAIProvider) Name() string  { return "capture" }
func (provider *captureAIProvider) Model() string { return "capture-v1" }
func (provider *captureAIProvider) Decide(
	_ context.Context,
	input ai.DecisionInput,
) (ai.Decision, map[string]any, error) {
	provider.input = input
	return ai.Decision{
		Category: "communication", Priority: "normal", Summary: "Ada sent a message.",
		Confidence: .93, ReasonCodes: []string{"direct_message"},
	}, map[string]any{"capture": true}, nil
}

func TestSettingsExposeNonSecretAIAnalysisProfile(t *testing.T) {
	apiServer := API{Config: config.Config{AI: config.AIConfig{
		Enabled: true, APIKey: "must-not-leak", Model: "gemini-3.5-flash-lite",
		PromptVersion: "dispatch-analysis-v3", MinConfidence: .75,
		Timeout: 5 * time.Second, MaxInputChars: 12000, SummaryMaxChars: 180,
		ThinkingLevel: "minimal",
	}}}
	response := httptest.NewRecorder()

	apiServer.settings(response, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	if strings.Contains(body, "must-not-leak") {
		t.Fatal("AI secret leaked from settings")
	}
	if !strings.Contains(body, `"min_confidence":0.75`) ||
		!strings.Contains(body, `"communication"`) {
		t.Fatalf("analysis profile is incomplete: %s", body)
	}
}

func TestAIPreviewRedactsUntrustedMetadata(t *testing.T) {
	provider := &captureAIProvider{}
	apiServer := API{
		Config: config.Config{AI: config.AIConfig{Enabled: true, Timeout: time.Second}},
		AI:     provider,
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/preview", strings.NewReader(`{
		"event_type":"telegram.message",
		"subject":"Message",
		"body":"Hello",
		"metadata":{"sender_username":"ada","access_token":"secret"}
	}`))

	apiServer.aiPreview(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if provider.input.Metadata["sender_username"] != "ada" {
		t.Fatalf("safe metadata was lost: %#v", provider.input.Metadata)
	}
	if _, ok := provider.input.Metadata["access_token"]; ok {
		t.Fatal("secret metadata reached the AI provider")
	}
	var payload struct {
		Data ai.Decision `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Category != "communication" {
		t.Fatalf("unexpected preview: %#v", payload.Data)
	}
}
