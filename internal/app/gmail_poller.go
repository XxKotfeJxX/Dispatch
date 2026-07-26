package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dispatch/internal/connectors"
	"dispatch/internal/ingress"
	"dispatch/internal/notification"
)

type gmailProfile struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    string `json:"historyId"`
}

type gmailHistoryResponse struct {
	History []struct {
		ID            string `json:"id"`
		MessagesAdded []struct {
			Message connectors.GmailMessage `json:"message"`
		} `json:"messagesAdded"`
	} `json:"history"`
	NextPageToken string `json:"nextPageToken"`
	HistoryID     string `json:"historyId"`
}

type gmailAPIError struct {
	StatusCode int
	Status     string
	Body       string
}

func (value *gmailAPIError) Error() string {
	if value.Body == "" {
		return "Gmail API returned " + value.Status
	}
	return fmt.Sprintf("Gmail API returned %s: %s", value.Status, value.Body)
}

func (worker *Worker) pollGmailConnection(ctx context.Context, connectionID string) error {
	connection, credentials, err := worker.loadGoogleConnection(ctx, connectionID)
	if err != nil {
		return err
	}

	historyID := strings.TrimSpace(connection.Config["history_id"])
	if historyID == "" {
		profile, err := worker.gmailProfile(ctx, credentials.AccessToken)
		if err != nil {
			return err
		}
		connection.Config["history_id"] = profile.HistoryID
		if err := worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config); err != nil {
			return err
		}
		return worker.Store.UpdateConnectorState(
			ctx, connection.ID, "connected", profile.EmailAddress, "", true,
		)
	}

	latestHistoryID, err := worker.consumeGmailHistory(ctx, connection, credentials, historyID)
	var apiError *gmailAPIError
	if errors.As(err, &apiError) && apiError.StatusCode == http.StatusNotFound {
		profile, profileErr := worker.gmailProfile(ctx, credentials.AccessToken)
		if profileErr != nil {
			return profileErr
		}
		connection.Config["history_id"] = profile.HistoryID
		if updateErr := worker.Store.UpdateConnectorConfig(
			ctx, connection.ID, connection.Config,
		); updateErr != nil {
			return updateErr
		}
		worker.Logger.Info("reset stale Gmail history cursor", "connection_id", connection.ID)
		return worker.Store.UpdateConnectorState(
			ctx, connection.ID, "connected", profile.EmailAddress, "", true,
		)
	}
	if err != nil {
		return err
	}
	if latestHistoryID != "" && latestHistoryID != historyID {
		connection.Config["history_id"] = latestHistoryID
		if err := worker.Store.UpdateConnectorConfig(ctx, connection.ID, connection.Config); err != nil {
			return err
		}
	}
	return worker.Store.UpdateConnectorState(ctx, connection.ID, "connected", "", "", true)
}

func (worker *Worker) consumeGmailHistory(
	ctx context.Context,
	connection connectors.Connection,
	credentials connectors.Credentials,
	startHistoryID string,
) (string, error) {
	pageToken, latestHistoryID := "", startHistoryID
	seen := map[string]bool{}
	for {
		values := url.Values{
			"startHistoryId": {startHistoryID},
			"historyTypes":   {"messageAdded"},
			"labelId":        {"INBOX"},
		}
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		var response gmailHistoryResponse
		if err := worker.gmailGet(
			ctx, credentials.AccessToken, "/gmail/v1/users/me/history?"+values.Encode(), &response,
		); err != nil {
			return "", err
		}
		if response.HistoryID != "" {
			latestHistoryID = response.HistoryID
		}
		for _, entry := range response.History {
			for _, added := range entry.MessagesAdded {
				messageID := added.Message.ID
				if messageID == "" || seen[messageID] {
					continue
				}
				seen[messageID] = true
				message, err := worker.gmailMessage(ctx, credentials.AccessToken, messageID)
				if err != nil {
					return "", err
				}
				mode := connection.Config["mode"]
				if !connectors.GmailMessageMatches(mode, message) {
					continue
				}
				if err := worker.enqueueGoogleEvent(
					ctx, connection, connectors.NormalizeGmailMessage(message),
				); err != nil {
					return "", err
				}
			}
		}
		pageToken = response.NextPageToken
		if pageToken == "" {
			return latestHistoryID, nil
		}
	}
}

func (worker *Worker) gmailProfile(ctx context.Context, accessToken string) (gmailProfile, error) {
	var profile gmailProfile
	err := worker.gmailGet(ctx, accessToken, "/gmail/v1/users/me/profile", &profile)
	if err == nil && profile.HistoryID == "" {
		err = errors.New("Gmail profile omitted historyId")
	}
	return profile, err
}

func (worker *Worker) gmailMessage(
	ctx context.Context,
	accessToken, messageID string,
) (connectors.GmailMessage, error) {
	values := url.Values{"format": {"full"}}
	var message connectors.GmailMessage
	err := worker.gmailGet(
		ctx, accessToken,
		"/gmail/v1/users/me/messages/"+url.PathEscape(messageID)+"?"+values.Encode(),
		&message,
	)
	return message, err
}

func (worker *Worker) gmailGet(
	ctx context.Context,
	accessToken, path string,
	target any,
) error {
	requestCtx, cancel := context.WithTimeout(ctx, worker.Config.ProviderTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestCtx, http.MethodGet,
		strings.TrimRight(worker.Config.Connectors.GmailAPIBase, "/")+path, nil,
	)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := (&http.Client{Timeout: worker.Config.ProviderTimeout}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return &gmailAPIError{
			StatusCode: response.StatusCode,
			Status:     response.Status,
			Body:       strings.TrimSpace(string(body)),
		}
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(target)
}

func (worker *Worker) googleCredentials(cipher []byte) (connectors.Credentials, error) {
	raw, err := ingress.DecryptSecret(worker.Config.Connectors.EncryptionKey, cipher)
	if err != nil {
		return connectors.Credentials{}, fmt.Errorf("decrypt Google credentials: %w", err)
	}
	var credentials connectors.Credentials
	if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
		return credentials, fmt.Errorf("decode Google credentials: %w", err)
	}
	if credentials.AccessToken == "" {
		return credentials, errors.New("Google OAuth access token is missing")
	}
	return credentials, nil
}

func (worker *Worker) refreshGoogleCredentials(
	ctx context.Context,
	credentials connectors.Credentials,
) (connectors.Credentials, bool, error) {
	if credentials.ExpiresAt == nil || time.Until(*credentials.ExpiresAt) >= 2*time.Minute {
		return credentials, false, nil
	}
	if credentials.RefreshToken == "" {
		return credentials, false, errors.New("Google OAuth refresh token is missing; reconnect the service")
	}
	provider, ok := connectors.OAuthSpec("google", worker.Config.Connectors)
	if !ok {
		return credentials, false, errors.New("Google OAuth is not configured")
	}
	values := url.Values{
		"client_id":     {provider.ClientID},
		"client_secret": {provider.ClientSecret},
		"refresh_token": {credentials.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	requestCtx, cancel := context.WithTimeout(ctx, worker.Config.ProviderTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestCtx, http.MethodPost, provider.TokenURL, strings.NewReader(values.Encode()),
	)
	if err != nil {
		return credentials, false, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := (&http.Client{Timeout: worker.Config.ProviderTimeout}).Do(request)
	if err != nil {
		return credentials, false, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return credentials, false, fmt.Errorf(
			"Google OAuth refresh returned %s: %s", response.Status, strings.TrimSpace(string(body)),
		)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return credentials, false, err
	}
	if token.AccessToken == "" {
		return credentials, false, errors.New("Google OAuth refresh omitted access_token")
	}
	credentials.AccessToken = token.AccessToken
	if token.TokenType != "" {
		credentials.TokenType = token.TokenType
	}
	if token.Scope != "" {
		credentials.Scope = token.Scope
	}
	if token.ExpiresIn > 0 {
		expiresAt := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
		credentials.ExpiresAt = &expiresAt
	}
	return credentials, true, nil
}

func (worker *Worker) saveGoogleCredentials(
	ctx context.Context,
	connectionID string,
	credentials connectors.Credentials,
) error {
	raw, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	cipher, err := ingress.EncryptSecret(
		worker.Config.Connectors.EncryptionKey, string(raw),
	)
	if err != nil {
		return err
	}
	return worker.Store.UpdateConnectorCredentials(
		ctx, connectionID, cipher, credentials.ExpiresAt,
	)
}

func (worker *Worker) enqueueGoogleEvent(
	ctx context.Context,
	connection connectors.Connection,
	event connectors.NormalizedEvent,
) error {
	digest := sha256.Sum256([]byte(connection.ID + ":" + event.ExternalID))
	item := notification.Notification{
		IdempotencyKey: "con_" + hex.EncodeToString(digest[:]),
		RecipientID:    connection.RecipientID,
		EventType:      event.EventType,
		Subject:        event.Subject,
		Body:           event.Body,
		Metadata:       connectors.ConnectionMetadata(connection, event.Metadata),
	}
	if _, err := worker.Store.CreateNotification(ctx, &item); err != nil {
		return err
	}
	if err := worker.Store.RecordConnectorEvent(
		ctx, connection.ID, event.ExternalID, item.ID, event.EventType,
	); err != nil {
		return err
	}
	return worker.Store.TouchConnectorEvent(ctx, connection.ID)
}
