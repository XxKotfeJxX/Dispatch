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
