package app

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dispatch/internal/connectors"
)

type googleChatSpace struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	SpaceType   string `json:"spaceType"`
	DisplayName string `json:"displayName"`
}

type googleChatSpacesResponse struct {
	Spaces        []googleChatSpace `json:"spaces"`
	NextPageToken string            `json:"nextPageToken"`
}

type googleChatMessage struct {
	Name           string `json:"name"`
	Text           string `json:"text"`
	FormattedText  string `json:"formattedText"`
	CreateTime     string `json:"createTime"`
	LastUpdateTime string `json:"lastUpdateTime"`
	DeletedTime    string `json:"deleteTime"`
	Sender         struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Type        string `json:"type"`
	} `json:"sender"`
	Attachments []struct {
		Name        string `json:"name"`
		ContentName string `json:"contentName"`
		ContentType string `json:"contentType"`
		Source      string `json:"source"`
	} `json:"attachment"`
}

type googleChatMessagesResponse struct {
	Messages      []googleChatMessage `json:"messages"`
	NextPageToken string              `json:"nextPageToken"`
}

func (worker *Worker) pollGoogleChat(ctx context.Context, connectionID string) error {
	connection, credentials, err := worker.loadGoogleConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	cursor := strings.TrimSpace(connection.Config["chat_cursor"])
	if cursor == "" {
		connection.Config["chat_cursor"] = startedAt.Format(time.RFC3339Nano)
		return worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config)
	}
	spaces, err := worker.googleChatSpaces(ctx, credentials.AccessToken)
	if err != nil {
		return err
	}
	for _, space := range spaces {
		if err := worker.pollGoogleChatSpace(
			ctx, connection, credentials, space, cursor,
		); err != nil {
			return err
		}
	}
	connection.Config["chat_cursor"] = startedAt.Format(time.RFC3339Nano)
	return worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config)
}

func (worker *Worker) googleChatSpaces(
	ctx context.Context,
	accessToken string,
) ([]googleChatSpace, error) {
	result := make([]googleChatSpace, 0)
	pageToken := ""
	for {
		values := url.Values{"pageSize": {"1000"}}
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		var response googleChatSpacesResponse
		if err := worker.googleJSON(
			ctx, http.MethodGet, worker.Config.Connectors.ChatAPIBase,
			"/spaces?"+values.Encode(), accessToken, nil, &response,
		); err != nil {
			return nil, err
		}
		result = append(result, response.Spaces...)
		pageToken = response.NextPageToken
		if pageToken == "" {
			return result, nil
		}
	}
}

func (worker *Worker) pollGoogleChatSpace(
	ctx context.Context,
	connection connectors.Connection,
	credentials connectors.Credentials,
	space googleChatSpace,
	cursor string,
) error {
	pageToken := ""
	for {
		values := url.Values{
			"pageSize": {"1000"},
			"filter":   {`createTime > "` + cursor + `"`},
			"orderBy":  {"ASC"},
		}
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		var response googleChatMessagesResponse
		if err := worker.googleJSON(
			ctx, http.MethodGet, worker.Config.Connectors.ChatAPIBase,
			"/"+strings.TrimPrefix(space.Name, "/")+"/messages?"+values.Encode(),
			credentials.AccessToken, nil, &response,
		); err != nil {
			return err
		}
		for _, message := range response.Messages {
			if message.Name == "" || message.CreateTime == "" {
				continue
			}
			if credentials.Values["provider_user_id"] != "" &&
				strings.HasSuffix(message.Sender.Name, "/"+credentials.Values["provider_user_id"]) {
				continue
			}
			if err := worker.enqueueGoogleEvent(
				ctx, connection, normalizeGoogleChatMessage(space, message),
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

func normalizeGoogleChatMessage(
	space googleChatSpace,
	message googleChatMessage,
) connectors.NormalizedEvent {
	sender := strings.TrimSpace(message.Sender.DisplayName)
	if sender == "" {
		sender = strings.TrimPrefix(message.Sender.Name, "users/")
	}
	if sender == "" {
		sender = "Google Chat"
	}
	spaceLabel := strings.TrimSpace(space.DisplayName)
	if spaceLabel == "" {
		switch space.SpaceType {
		case "DIRECT_MESSAGE":
			spaceLabel = "Direct message"
		case "GROUP_CHAT":
			spaceLabel = "Group chat"
		default:
			spaceLabel = "Google Chat"
		}
	}
	text := message.Text
	if strings.TrimSpace(text) == "" {
		text = message.FormattedText
	}
	body := truncateGoogleText(text, 12000)
	attachmentNames := make([]string, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		label := strings.TrimSpace(attachment.ContentName)
		if label == "" {
			label = strings.TrimSpace(attachment.Name)
		}
		if attachment.ContentType != "" {
			label += " (" + attachment.ContentType + ")"
		}
		if label != "" {
			attachmentNames = append(attachmentNames, label)
		}
	}
	if len(attachmentNames) > 0 {
		body += "\n\nAttachments:\n- " + strings.Join(attachmentNames, "\n- ")
	}
	return connectors.NormalizedEvent{
		ExternalID: "chat:" + message.Name,
		EventType:  "google.chat.message",
		Subject:    sender + " in " + spaceLabel,
		Body:       strings.TrimSpace(body),
		Metadata: map[string]any{
			"connector":   "google",
			"provider":    "chat",
			"message_id":  message.Name,
			"space_id":    space.Name,
			"space_name":  spaceLabel,
			"sender":      sender,
			"sender_id":   strings.TrimPrefix(message.Sender.Name, "users/"),
			"space_type":  space.SpaceType,
			"created":     message.CreateTime,
			"attachments": attachmentNames,
		},
	}
}
