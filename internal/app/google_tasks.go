package app

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dispatch/internal/connectors"
)

type googleTaskList struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type googleTaskListsResponse struct {
	Items         []googleTaskList `json:"items"`
	NextPageToken string           `json:"nextPageToken"`
}

type googleTask struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Notes     string `json:"notes"`
	Status    string `json:"status"`
	Due       string `json:"due"`
	Completed string `json:"completed"`
	Updated   string `json:"updated"`
	Deleted   bool   `json:"deleted"`
	Hidden    bool   `json:"hidden"`
	Links     []struct {
		Type        string `json:"type"`
		Description string `json:"description"`
		Link        string `json:"link"`
	} `json:"links"`
}

type googleTasksResponse struct {
	Items         []googleTask `json:"items"`
	NextPageToken string       `json:"nextPageToken"`
}

func (worker *Worker) pollGoogleTasks(ctx context.Context, connectionID string) error {
	connection, credentials, err := worker.loadGoogleConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	cursor := strings.TrimSpace(connection.Config["tasks_cursor"])
	if cursor == "" {
		connection.Config["tasks_cursor"] = startedAt.Format(time.RFC3339Nano)
		return worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config)
	}
	taskLists, err := worker.googleTaskLists(ctx, credentials.AccessToken)
	if err != nil {
		return err
	}
	for _, list := range taskLists {
		if err := worker.pollGoogleTaskList(
			ctx, connection, credentials, list, cursor,
		); err != nil {
			return err
		}
	}
	connection.Config["tasks_cursor"] = startedAt.Format(time.RFC3339Nano)
	return worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config)
}

func (worker *Worker) googleTaskLists(
	ctx context.Context,
	accessToken string,
) ([]googleTaskList, error) {
	result := make([]googleTaskList, 0)
	pageToken := ""
	for {
		values := url.Values{"maxResults": {"100"}}
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		var response googleTaskListsResponse
		if err := worker.googleJSON(
			ctx, http.MethodGet, worker.Config.Connectors.TasksAPIBase,
			"/users/@me/lists?"+values.Encode(), accessToken, nil, &response,
		); err != nil {
			return nil, err
		}
		result = append(result, response.Items...)
		pageToken = response.NextPageToken
		if pageToken == "" {
			return result, nil
		}
	}
}

func (worker *Worker) pollGoogleTaskList(
	ctx context.Context,
	connection connectors.Connection,
	credentials connectors.Credentials,
	list googleTaskList,
	cursor string,
) error {
	pageToken := ""
	for {
		values := url.Values{
			"maxResults":    {"100"},
			"showCompleted": {"true"},
			"showDeleted":   {"true"},
			"showHidden":    {"true"},
			"showAssigned":  {"true"},
			"updatedMin":    {cursor},
		}
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		var response googleTasksResponse
		if err := worker.googleJSON(
			ctx, http.MethodGet, worker.Config.Connectors.TasksAPIBase,
			"/lists/"+url.PathEscape(list.ID)+"/tasks?"+values.Encode(),
			credentials.AccessToken, nil, &response,
		); err != nil {
			return err
		}
		for _, task := range response.Items {
			if task.ID == "" || task.Updated == "" {
				continue
			}
			if err := worker.enqueueGoogleEvent(
				ctx, connection, normalizeGoogleTask(list, task),
			); err != nil {
				return err
			}
		}
		pageToken = response.NextPageToken
		if pageToken == "" {
			return nil
		}
	}
}

func normalizeGoogleTask(list googleTaskList, task googleTask) connectors.NormalizedEvent {
	eventType := "google.tasks.updated"
	statusLabel := "Task updated"
	if task.Deleted {
		eventType, statusLabel = "google.tasks.deleted", "Task deleted"
	} else if task.Status == "completed" {
		eventType, statusLabel = "google.tasks.completed", "Task completed"
	}
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = "(untitled task)"
	}
	body := []string{"List: " + list.Title, "Status: " + task.Status}
	if task.Due != "" {
		body = append(body, "Due: "+task.Due)
	}
	if task.Completed != "" {
		body = append(body, "Completed: "+task.Completed)
	}
	if notes := truncateGoogleText(task.Notes, 8000); notes != "" {
		body = append(body, "", notes)
	}
	return connectors.NormalizedEvent{
		ExternalID: "tasks:" + list.ID + ":" + task.ID + ":" + task.Updated,
		EventType:  eventType,
		Subject:    statusLabel + ": " + title,
		Body:       strings.Join(body, "\n"),
		Metadata: map[string]any{
			"connector": "google",
			"provider":  "tasks",
			"list_id":   list.ID,
			"list_name": list.Title,
			"task_id":   task.ID,
			"status":    task.Status,
			"due":       task.Due,
			"updated":   task.Updated,
		},
	}
}
