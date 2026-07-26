package app

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dispatch/internal/connectors"
)

type googleCalendarEvent struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Created     string `json:"created"`
	Updated     string `json:"updated"`
	HTMLLink    string `json:"htmlLink"`
	HangoutLink string `json:"hangoutLink"`
	Start       struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
		TimeZone string `json:"timeZone"`
	} `json:"start"`
	End struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
		TimeZone string `json:"timeZone"`
	} `json:"end"`
	Creator struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
	} `json:"creator"`
	Organizer struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
	} `json:"organizer"`
	Attendees []struct {
		Email          string `json:"email"`
		DisplayName    string `json:"displayName"`
		ResponseStatus string `json:"responseStatus"`
		Self           bool   `json:"self"`
	} `json:"attendees"`
}

type googleCalendarEventsResponse struct {
	Items         []googleCalendarEvent `json:"items"`
	NextPageToken string                `json:"nextPageToken"`
}

func (worker *Worker) pollGoogleCalendar(ctx context.Context, connectionID string) error {
	connection, credentials, err := worker.loadGoogleConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	cursor := strings.TrimSpace(connection.Config["calendar_cursor"])
	if cursor == "" {
		connection.Config["calendar_cursor"] = startedAt.Format(time.RFC3339Nano)
		if err := worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config); err != nil {
			return err
		}
		return worker.pollGoogleCalendarReminders(ctx, connection, credentials, startedAt)
	}
	since, err := time.Parse(time.RFC3339Nano, cursor)
	if err != nil {
		since = startedAt
	}
	values := url.Values{
		"updatedMin":   {since.Add(-2 * time.Second).Format(time.RFC3339Nano)},
		"showDeleted":  {"true"},
		"singleEvents": {"true"},
		"maxResults":   {"250"},
	}
	pageToken := ""
	for {
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		var response googleCalendarEventsResponse
		path := "/calendars/primary/events?" + values.Encode()
		if err := worker.googleJSON(
			ctx, http.MethodGet, worker.Config.Connectors.CalendarAPIBase,
			path, credentials.AccessToken, nil, &response,
		); err != nil {
			return err
		}
		for _, event := range response.Items {
			if event.ID == "" || event.Updated == "" {
				continue
			}
			normalized := normalizeGoogleCalendarEvent(event)
			if err := worker.enqueueGoogleEvent(ctx, connection, normalized); err != nil {
				return err
			}
		}
		pageToken = response.NextPageToken
		if pageToken == "" {
			break
		}
	}
	if err := worker.pollGoogleCalendarReminders(
		ctx, connection, credentials, startedAt,
	); err != nil {
		return err
	}
	connection.Config["calendar_cursor"] = startedAt.Format(time.RFC3339Nano)
	return worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config)
}

func (worker *Worker) pollGoogleCalendarReminders(
	ctx context.Context,
	connection connectors.Connection,
	credentials connectors.Credentials,
	now time.Time,
) error {
	minutes := 15
	if parsed, err := strconv.Atoi(connection.Config["calendar_reminder_minutes"]); err == nil &&
		parsed >= 5 && parsed <= 1440 {
		minutes = parsed
	}
	values := url.Values{
		"timeMin":      {now.Format(time.RFC3339Nano)},
		"timeMax":      {now.Add(time.Duration(minutes) * time.Minute).Format(time.RFC3339Nano)},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
		"maxResults":   {"100"},
	}
	var response googleCalendarEventsResponse
	if err := worker.googleJSON(
		ctx, http.MethodGet, worker.Config.Connectors.CalendarAPIBase,
		"/calendars/primary/events?"+values.Encode(),
		credentials.AccessToken, nil, &response,
	); err != nil {
		return err
	}
	for _, event := range response.Items {
		if event.ID == "" || event.Status == "cancelled" {
			continue
		}
		start := calendarEventTime(event.Start.DateTime, event.Start.Date)
		normalized := normalizeGoogleCalendarEvent(event)
		normalized.ExternalID = "calendar-reminder:" + event.ID + ":" + start
		normalized.EventType = "google.calendar.reminder"
		normalized.Subject = fmt.Sprintf("Upcoming: %s", calendarSummary(event))
		if err := worker.enqueueGoogleEvent(ctx, connection, normalized); err != nil {
			return err
		}
	}
	return nil
}

func normalizeGoogleCalendarEvent(event googleCalendarEvent) connectors.NormalizedEvent {
	eventType := "google.calendar.updated"
	if event.Status == "cancelled" {
		eventType = "google.calendar.cancelled"
	} else if event.Created != "" && event.Created == event.Updated {
		eventType = "google.calendar.created"
	}
	summary := calendarSummary(event)
	start := calendarEventTime(event.Start.DateTime, event.Start.Date)
	end := calendarEventTime(event.End.DateTime, event.End.Date)
	bodyParts := []string{"Start: " + start}
	if end != "" {
		bodyParts = append(bodyParts, "End: "+end)
	}
	if event.Location != "" {
		bodyParts = append(bodyParts, "Location: "+event.Location)
	}
	if event.HangoutLink != "" {
		bodyParts = append(bodyParts, "Google Meet: "+event.HangoutLink)
	}
	if description := truncateGoogleText(event.Description, 8000); description != "" {
		bodyParts = append(bodyParts, "", description)
	}
	if event.HTMLLink != "" {
		bodyParts = append(bodyParts, "", "Open in Google Calendar: "+event.HTMLLink)
	}
	return connectors.NormalizedEvent{
		ExternalID: "calendar:" + event.ID + ":" + event.Updated + ":" + event.Status,
		EventType:  eventType,
		Subject:    summary,
		Body:       strings.Join(bodyParts, "\n"),
		Metadata: map[string]any{
			"connector":   "google",
			"provider":    "calendar",
			"event_id":    event.ID,
			"status":      event.Status,
			"start":       start,
			"end":         end,
			"location":    event.Location,
			"meet_url":    event.HangoutLink,
			"url":         event.HTMLLink,
			"updated":     event.Updated,
			"event_title": summary,
		},
	}
}

func calendarSummary(event googleCalendarEvent) string {
	if value := strings.TrimSpace(event.Summary); value != "" {
		return value
	}
	return "(untitled calendar event)"
}

func calendarEventTime(dateTime, date string) string {
	if dateTime != "" {
		return dateTime
	}
	return date
}

func truncateGoogleText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "\n[truncated by Dispatch]"
}
