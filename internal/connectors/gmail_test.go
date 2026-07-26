package connectors

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGmailMessageMatches(t *testing.T) {
	message := GmailMessage{LabelIDs: []string{"INBOX", "UNREAD"}}
	if !GmailMessageMatches(GmailModeInbox, message) {
		t.Fatal("inbox message should match inbox mode")
	}
	if !GmailMessageMatches(GmailModeUnread, message) {
		t.Fatal("unread message should match unread mode")
	}
	if GmailMessageMatches(GmailModeImportant, message) {
		t.Fatal("non-important message must not match important mode")
	}
	message.LabelIDs = []string{"UNREAD"}
	if GmailMessageMatches(GmailModeInbox, message) {
		t.Fatal("message outside inbox must not match")
	}
}

func TestNormalizeGmailMessage(t *testing.T) {
	message := GmailMessage{
		ID: "msg-1", ThreadID: "thread-1", LabelIDs: []string{"INBOX"},
	}
	message.Payload.Headers = []GmailHeader{
		{Name: "From", Value: "Alice <alice@example.com>"},
		{Name: "Subject", Value: "Build completed"},
	}
	message.Payload.MimeType = "text/plain"
	message.Payload.Body.Data = base64.RawURLEncoding.EncodeToString(
		[]byte("The production build completed successfully."),
	)
	event := NormalizeGmailMessage(message)
	if event.ExternalID != "msg-1" || event.EventType != "gmail.message.received" {
		t.Fatalf("unexpected identity: %#v", event)
	}
	if !strings.Contains(event.Subject, "Build completed") ||
		!strings.Contains(event.Body, "production build completed") ||
		!strings.Contains(event.Body, "mail.google.com") {
		t.Fatalf("unexpected normalized event: %#v", event)
	}
}

func TestGmailMessageTextPrefersPlainTextAndDoesNotFetchAttachments(t *testing.T) {
	message := GmailMessage{}
	plain := GmailPart{MimeType: "text/plain"}
	plain.Body.Data = base64.RawURLEncoding.EncodeToString([]byte("Readable content"))
	htmlPart := GmailPart{MimeType: "text/html"}
	htmlPart.Body.Data = base64.RawURLEncoding.EncodeToString([]byte("<b>Duplicate content</b>"))
	attachment := GmailPart{MimeType: "application/pdf"}
	attachment.Filename = "quarterly-report.pdf"
	attachment.Body.AttachmentID = "not-fetched"
	attachment.Body.Size = 2048
	message.Payload.Parts = []GmailPart{plain, htmlPart, attachment}

	text, truncated := GmailMessageText(message)
	if truncated || text != "Readable content" {
		t.Fatalf("unexpected extracted text: %q, truncated=%v", text, truncated)
	}
	attachments := GmailAttachments(message)
	if len(attachments) != 1 || attachments[0].Filename != "quarterly-report.pdf" ||
		attachments[0].MimeType != "application/pdf" || attachments[0].Size != 2048 {
		t.Fatalf("unexpected attachment metadata: %#v", attachments)
	}
	event := NormalizeGmailMessage(message)
	if !strings.Contains(event.Body, "quarterly-report.pdf") ||
		!strings.Contains(event.Body, "application/pdf") {
		t.Fatalf("attachment context missing from AI body: %q", event.Body)
	}
}
