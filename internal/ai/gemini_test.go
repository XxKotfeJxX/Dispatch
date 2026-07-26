package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dispatch/internal/config"
)

func testAIConfig() config.AIConfig {
	return config.AIConfig{
		APIKey: "test-key", Model: "gemini-3.5-flash-lite",
		MaxInputChars: 1000, MaxOutputTokens: 512, SummaryMaxChars: 180,
		ThinkingLevel: "minimal", Timeout: 5 * time.Second,
	}
}

func TestGeminiUsesGuardedStructuredAnalysisRequest(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-goog-api-key") != "test-key" {
			t.Error("missing Gemini API key")
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"output_text": `{"category":"communication","priority":"normal","summary":"Ada sent a message.","confidence":0.94,"reason_codes":["direct_message"]}`,
		})
	}))
	defer server.Close()

	provider := NewGemini(testAIConfig(), server.Client())
	provider.endpoint = server.URL
	decision, _, err := provider.Decide(context.Background(), DecisionInput{
		EventType: "telegram.message", Subject: "Message",
		Body: strings.Repeat("hello ", 1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Category != "communication" || decision.Priority != "normal" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if requestBody["system_instruction"] == "" || requestBody["store"] != false {
		t.Fatalf("guardrails or zero-retention request missing: %#v", requestBody)
	}
	generation, _ := requestBody["generation_config"].(map[string]any)
	if generation["thinking_level"] != "minimal" ||
		generation["max_output_tokens"] != float64(512) {
		t.Fatalf("generation config was not applied: %#v", generation)
	}
	input, _ := requestBody["input"].(string)
	if len([]rune(input)) > 1200 {
		t.Fatalf("bounded input unexpectedly large: %d", len([]rune(input)))
	}
}

func TestGeminiReturnsSpecificProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"error":{"message":"model not found"}}`))
	}))
	defer server.Close()

	provider := NewGemini(testAIConfig(), server.Client())
	provider.endpoint = server.URL
	_, _, err := provider.Decide(context.Background(), DecisionInput{Subject: "Test"})
	if reason := FallbackReason(err); reason != "provider_error_http_404" {
		t.Fatalf("unexpected fallback reason %q for %v", reason, err)
	}
}
