package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeGoogleCalendarEventIncludesMeetContext(t *testing.T) {
	event := googleCalendarEvent{
		ID: "event-1", Status: "confirmed", Summary: "Planning",
		Updated: "2026-07-26T10:00:00Z", HangoutLink: "https://meet.google.com/abc-defg-hij",
	}
	event.Start.DateTime = "2026-07-27T10:00:00Z"
	event.End.DateTime = "2026-07-27T10:30:00Z"
	normalized := normalizeGoogleCalendarEvent(event)
	if normalized.EventType != "google.calendar.updated" ||
		!strings.Contains(normalized.Body, "meet.google.com") {
		t.Fatalf("unexpected calendar event: %#v", normalized)
	}
}

func TestNormalizeGoogleDriveActivity(t *testing.T) {
	activity := googleDriveActivity{
		PrimaryActionDetail: map[string]json.RawMessage{"comment": json.RawMessage(`{}`)},
		Timestamp:           "2026-07-26T10:00:00Z",
		Targets: []map[string]any{{
			"driveItem": map[string]any{"name": "items/file-1", "title": "Budget.xlsx"},
		}},
	}
	normalized := normalizeGoogleDriveActivity(activity)
	if normalized.EventType != "google.drive.comment" ||
		!strings.Contains(normalized.Subject, "Budget.xlsx") ||
		!strings.Contains(normalized.Body, "drive.google.com/open?id=file-1") {
		t.Fatalf("unexpected Drive event: %#v", normalized)
	}
}

func TestNormalizeGoogleTaskAndChatMessage(t *testing.T) {
	task := googleTask{
		ID: "task-1", Title: "Submit report", Status: "needsAction",
		Due: "2026-07-27T00:00:00Z", Updated: "2026-07-26T10:00:00Z",
	}
	taskEvent := normalizeGoogleTask(googleTaskList{ID: "list-1", Title: "Work"}, task)
	if !strings.Contains(taskEvent.Subject, "Submit report") ||
		!strings.Contains(taskEvent.Body, "Due:") {
		t.Fatalf("unexpected Tasks event: %#v", taskEvent)
	}

	space := googleChatSpace{Name: "spaces/one", SpaceType: "DIRECT_MESSAGE"}
	message := googleChatMessage{
		Name: "spaces/one/messages/two", Text: "Can you review this?",
		CreateTime: "2026-07-26T10:00:00Z",
	}
	message.Sender.DisplayName = "Alice"
	chatEvent := normalizeGoogleChatMessage(space, message)
	if chatEvent.EventType != "google.chat.message" ||
		!strings.Contains(chatEvent.Subject, "Alice") ||
		!strings.Contains(chatEvent.Body, "review") {
		t.Fatalf("unexpected Chat event: %#v", chatEvent)
	}
}
