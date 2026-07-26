package config

import (
	"strings"
	"testing"
)

func TestConsoleAuthDisabledAllowsLocalModeWithoutStrongAPIKey(t *testing.T) {
	t.Setenv("CONSOLE_AUTH_ENABLED", "false")
	t.Setenv("API_KEY", "short")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ConsoleAuthEnabled {
		t.Fatal("ConsoleAuthEnabled = true, want false")
	}
}

func TestConsoleAuthEnabledRequiresStrongAPIKey(t *testing.T) {
	t.Setenv("CONSOLE_AUTH_ENABLED", "true")
	t.Setenv("API_KEY", "short")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "API_KEY") {
		t.Fatalf("Load() error = %v, want API_KEY validation error", err)
	}
}

func TestAIConfigUsesSafeAnalysisDefaults(t *testing.T) {
	t.Setenv("AI_ENABLED", "false")
	for _, name := range []string{
		"AI_TIMEOUT", "AI_MAX_INPUT_CHARS", "AI_SUMMARY_MAX_CHARS",
		"AI_THINKING_LEVEL", "AI_PROMPT_VERSION",
	} {
		t.Setenv(name, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Timeout.String() != "12s" || cfg.AI.MaxInputChars != 12000 ||
		cfg.AI.SummaryMaxChars != 180 || cfg.AI.ThinkingLevel != "minimal" ||
		cfg.AI.PromptVersion != "dispatch-analysis-v3" {
		t.Fatalf("unexpected AI defaults: %#v", cfg.AI)
	}
}

func TestAIConfigRejectsUnknownThinkingLevel(t *testing.T) {
	t.Setenv("AI_ENABLED", "false")
	t.Setenv("AI_THINKING_LEVEL", "maximum")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "AI_THINKING_LEVEL") {
		t.Fatalf("Load() error = %v, want AI_THINKING_LEVEL validation error", err)
	}
}
