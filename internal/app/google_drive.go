package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dispatch/internal/connectors"
)

type googleDriveActivity struct {
	PrimaryActionDetail map[string]json.RawMessage `json:"primaryActionDetail"`
	Actors              []map[string]any           `json:"actors"`
	Targets             []map[string]any           `json:"targets"`
	Timestamp           string                     `json:"timestamp"`
	TimeRange           struct {
		StartTime string `json:"startTime"`
		EndTime   string `json:"endTime"`
	} `json:"timeRange"`
}

type googleDriveActivityResponse struct {
	Activities    []googleDriveActivity `json:"activities"`
	NextPageToken string                `json:"nextPageToken"`
}

func (worker *Worker) pollGoogleDrive(ctx context.Context, connectionID string) error {
	connection, credentials, err := worker.loadGoogleConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	cursor := strings.TrimSpace(connection.Config["drive_cursor"])
	if cursor == "" {
		connection.Config["drive_cursor"] = startedAt.Format(time.RFC3339Nano)
		return worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config)
	}
	pageToken := ""
	for {
		request := map[string]any{
			"ancestorName": "items/root",
			"filter":       fmt.Sprintf(`time > "%s"`, cursor),
			"pageSize":     100,
			"consolidationStrategy": map[string]any{
				"none": map[string]any{},
			},
		}
		if pageToken != "" {
			request["pageToken"] = pageToken
		}
		var response googleDriveActivityResponse
		if err := worker.googleJSON(
			ctx, http.MethodPost, worker.Config.Connectors.DriveActivityBase,
			"/activity:query", credentials.AccessToken, request, &response,
		); err != nil {
			return err
		}
		for _, activity := range response.Activities {
			event := normalizeGoogleDriveActivity(activity)
			if event.ExternalID == "" {
				continue
			}
			if err := worker.enqueueGoogleEvent(ctx, connection, event); err != nil {
				return err
			}
		}
		pageToken = response.NextPageToken
		if pageToken == "" {
			break
		}
	}
	connection.Config["drive_cursor"] = startedAt.Format(time.RFC3339Nano)
	return worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config)
}

func normalizeGoogleDriveActivity(activity googleDriveActivity) connectors.NormalizedEvent {
	raw, _ := json.Marshal(activity)
	if len(raw) == 0 {
		return connectors.NormalizedEvent{}
	}
	digest := sha256.Sum256(raw)
	action := googleDriveAction(activity.PrimaryActionDetail)
	titles, itemIDs := googleDriveTargets(activity.Targets)
	target := "Google Drive item"
	if len(titles) > 0 {
		target = strings.Join(titles, ", ")
	}
	when := activity.Timestamp
	if when == "" {
		when = activity.TimeRange.EndTime
	}
	actor := googleDriveActor(activity.Actors)
	body := []string{
		"Action: " + action,
		"Item: " + target,
	}
	if actor != "" {
		body = append(body, "Actor: "+actor)
	}
	if when != "" {
		body = append(body, "Time: "+when)
	}
	if len(itemIDs) > 0 {
		body = append(body, "", "Open in Google Drive: https://drive.google.com/open?id="+itemIDs[0])
	}
	return connectors.NormalizedEvent{
		ExternalID: "drive:" + hex.EncodeToString(digest[:]),
		EventType:  "google.drive." + action,
		Subject:    googleDriveActionLabel(action) + ": " + target,
		Body:       strings.Join(body, "\n"),
		Metadata: map[string]any{
			"connector": "google",
			"provider":  "drive",
			"action":    action,
			"items":     titles,
			"item_ids":  itemIDs,
			"actor":     actor,
			"timestamp": when,
		},
	}
}

func googleDriveAction(detail map[string]json.RawMessage) string {
	preferred := []string{
		"comment", "permissionChange", "create", "edit", "rename", "move",
		"delete", "restore", "dlpChange", "reference",
	}
	for _, key := range preferred {
		if _, ok := detail[key]; ok {
			return strings.ToLower(key)
		}
	}
	for key := range detail {
		return strings.ToLower(key)
	}
	return "activity"
}

func googleDriveActionLabel(action string) string {
	labels := map[string]string{
		"comment":          "Drive comment",
		"permissionchange": "Drive access changed",
		"create":           "Drive item created",
		"edit":             "Drive item edited",
		"rename":           "Drive item renamed",
		"move":             "Drive item moved",
		"delete":           "Drive item deleted",
		"restore":          "Drive item restored",
	}
	if label := labels[action]; label != "" {
		return label
	}
	return "Drive activity"
}

func googleDriveTargets(targets []map[string]any) ([]string, []string) {
	titles, ids := make([]string, 0), make([]string, 0)
	for _, target := range targets {
		if item, ok := target["driveItem"].(map[string]any); ok {
			if title, ok := item["title"].(string); ok && strings.TrimSpace(title) != "" {
				titles = append(titles, strings.TrimSpace(title))
			}
			if name, ok := item["name"].(string); ok {
				if id := strings.TrimPrefix(name, "items/"); id != "" {
					ids = append(ids, id)
				}
			}
		}
		if comment, ok := target["fileComment"].(map[string]any); ok {
			if parent, ok := comment["parent"].(map[string]any); ok {
				if title, ok := parent["title"].(string); ok && strings.TrimSpace(title) != "" {
					titles = append(titles, strings.TrimSpace(title))
				}
				if name, ok := parent["name"].(string); ok {
					if id := strings.TrimPrefix(name, "items/"); id != "" {
						ids = append(ids, id)
					}
				}
			}
		}
	}
	return uniqueGoogleStrings(titles), uniqueGoogleStrings(ids)
}

func googleDriveActor(actors []map[string]any) string {
	for _, actor := range actors {
		user, ok := actor["user"].(map[string]any)
		if !ok {
			continue
		}
		known, ok := user["knownUser"].(map[string]any)
		if !ok {
			if _, deleted := user["deletedUser"]; deleted {
				return "Deleted user"
			}
			continue
		}
		if current, _ := known["isCurrentUser"].(bool); current {
			return "You"
		}
		if name, ok := known["personName"].(string); ok {
			return name
		}
	}
	return ""
}

func uniqueGoogleStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
