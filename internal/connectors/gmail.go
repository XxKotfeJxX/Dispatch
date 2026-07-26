package connectors

import (
	"encoding/base64"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	GmailModeInbox     = "inbox"
	GmailModeUnread    = "unread"
	GmailModeImportant = "important"
)

type GmailHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type GmailMessage struct {
	ID           string    `json:"id"`
	ThreadID     string    `json:"threadId"`
	LabelIDs     []string  `json:"labelIds"`
	InternalDate string    `json:"internalDate"`
	Payload      GmailPart `json:"payload"`
}

// GmailPart is recursive because Gmail represents multipart messages as a tree.
// Attachments are never fetched; only inline text parts in the message response
// are considered.
type GmailPart struct {
	MimeType string        `json:"mimeType"`
	Filename string        `json:"filename"`
	Headers  []GmailHeader `json:"headers"`
	Body     struct {
		AttachmentID string `json:"attachmentId"`
		Data         string `json:"data"`
		Size         int    `json:"size"`
	} `json:"body"`
	Parts []GmailPart `json:"parts"`
}

const (
	gmailBodyLimit       = 12_000
	gmailAttachmentLimit = 50
	gmailFilenameLimit   = 256
)

var gmailHTMLTag = regexp.MustCompile(`<[^>]+>`)

type GmailAttachment struct {
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Size     int    `json:"size"`
}

func GmailMessageText(message GmailMessage) (string, bool) {
	plain, rich := make([]string, 0), make([]string, 0)
	collectGmailText(message.Payload, &plain, &rich)
	parts := plain
	if len(parts) == 0 {
		parts = rich
	}
	text := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if len(text) <= gmailBodyLimit {
		return text, false
	}
	text = text[:gmailBodyLimit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return strings.TrimSpace(text) + "\n\n[Message content truncated by Dispatch]", true
}

func collectGmailText(part GmailPart, plain, rich *[]string) {
	mimeType := strings.ToLower(strings.TrimSpace(part.MimeType))
	if part.Body.Data != "" && (mimeType == "text/plain" || mimeType == "text/html") {
		decoded, err := base64.RawURLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			decoded, err = base64.URLEncoding.DecodeString(part.Body.Data)
		}
		if err == nil {
			text := strings.TrimSpace(strings.ToValidUTF8(string(decoded), "[invalid utf-8]"))
			if mimeType == "text/html" {
				text = strings.TrimSpace(html.UnescapeString(gmailHTMLTag.ReplaceAllString(text, " ")))
				text = strings.Join(strings.Fields(text), " ")
				if text != "" {
					*rich = append(*rich, text)
				}
			} else if text != "" {
				*plain = append(*plain, text)
			}
		}
	}
	for _, child := range part.Parts {
		collectGmailText(child, plain, rich)
	}
}

func GmailAttachments(message GmailMessage) []GmailAttachment {
	result := make([]GmailAttachment, 0)
	collectGmailAttachments(message.Payload, &result)
	return result
}

func collectGmailAttachments(part GmailPart, result *[]GmailAttachment) {
	filename := safeGmailFilename(part.Filename)
	if filename != "" && len(*result) < gmailAttachmentLimit {
		*result = append(*result, GmailAttachment{
			Filename: filename,
			MimeType: strings.TrimSpace(part.MimeType),
			Size:     part.Body.Size,
		})
	}
	for _, child := range part.Parts {
		collectGmailAttachments(child, result)
	}
}

func safeGmailFilename(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= gmailFilenameLimit {
		return value
	}
	value = value[:gmailFilenameLimit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value) + "..."
}

func (message GmailMessage) Headers() []GmailHeader {
	return message.Payload.Headers
}

func GmailMessageMatches(mode string, message GmailMessage) bool {
	labels := make(map[string]bool, len(message.LabelIDs))
	for _, label := range message.LabelIDs {
		labels[label] = true
	}
	if !labels["INBOX"] {
		return false
	}
	switch mode {
	case GmailModeUnread:
		return labels["UNREAD"]
	case GmailModeImportant:
		return labels["IMPORTANT"]
	default:
		return true
	}
}

func NormalizeGmailMessage(message GmailMessage) NormalizedEvent {
	from := gmailHeader(message, "From")
	if from == "" {
		from = "Unknown sender"
	}
	subject := gmailHeader(message, "Subject")
	if subject == "" {
		subject = "(no subject)"
	}
	link := "https://mail.google.com/mail/u/0/#inbox/" + message.ID
	messageText, truncated := GmailMessageText(message)
	attachments := GmailAttachments(message)
	body := "Open this message in Gmail:\n" + link
	if messageText != "" {
		body = messageText + "\n\n" + body
	}
	if len(attachments) > 0 {
		lines := make([]string, 0, len(attachments))
		for _, attachment := range attachments {
			description := attachment.Filename
			if attachment.MimeType != "" {
				description += " (" + attachment.MimeType + ")"
			}
			lines = append(lines, description)
		}
		body += "\n\nAttachments:\n- " + strings.Join(lines, "\n- ")
	}
	return NormalizedEvent{
		ExternalID: message.ID,
		EventType:  "gmail.message.received",
		Subject:    fmt.Sprintf("%s - %s", from, subject),
		Body:       body,
		Metadata: map[string]any{
			"connector":      "google",
			"provider":       "gmail",
			"message_id":     message.ID,
			"thread_id":      message.ThreadID,
			"from":           from,
			"to":             gmailHeader(message, "To"),
			"subject":        subject,
			"date":           gmailHeader(message, "Date"),
			"labels":         message.LabelIDs,
			"internal_date":  message.InternalDate,
			"body_truncated": truncated,
			"attachments":    attachments,
			"url":            link,
		},
	}
}

func gmailHeader(message GmailMessage, name string) string {
	for _, header := range message.Headers() {
		if strings.EqualFold(header.Name, name) {
			return strings.TrimSpace(header.Value)
		}
	}
	return ""
}
