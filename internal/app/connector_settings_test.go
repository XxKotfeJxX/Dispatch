package app

import (
	"testing"

	"dispatch/internal/connectors"
)

func TestEditableConnectorConfigPreservesProviderState(t *testing.T) {
	config, err := editableConnectorConfig("google", map[string]string{
		"modules": "gmail,calendar", "mode": "inbox",
		"calendar_reminder_minutes": "15", "gmail_history_id": "123",
	}, map[string]string{
		"modules": "gmail,calendar", "mode": "important",
		"calendar_reminder_minutes": "30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config["mode"] != connectors.GmailModeImportant ||
		config["calendar_reminder_minutes"] != "30" ||
		config["gmail_history_id"] != "123" {
		t.Fatalf("unexpected edited config: %#v", config)
	}
}

func TestEditableConnectorConfigRejectsInvalidMode(t *testing.T) {
	if _, err := editableConnectorConfig(
		"discord", map[string]string{"guild_id": "1"}, map[string]string{"mode": "private_account"},
	); err == nil {
		t.Fatal("expected invalid Discord mode to be rejected")
	}
}
