package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"dispatch/internal/connectors"
	"dispatch/internal/ingress"
	"dispatch/internal/notification"
)

func (api *API) listConnectors(writer http.ResponseWriter, request *http.Request) {
	connections, err := api.Store.ListConnectorConnections(request.Context())
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": connectors.Catalog(api.Config.Connectors), "connections": connections,
	})
}

func (api *API) createConnectorConnection(writer http.ResponseWriter, request *http.Request) {
	var input connectors.CreateInput
	if !decode(writer, request, &input) {
		return
	}
	input.ConnectorID = strings.ToLower(strings.TrimSpace(input.ConnectorID))
	input.Name = strings.TrimSpace(input.Name)
	input.RecipientID = strings.TrimSpace(input.RecipientID)
	if input.Name == "" || input.RecipientID == "" {
		writeError(writer, http.StatusBadRequest, "validation_error", "name and recipient_id are required")
		return
	}
	if input.Config == nil {
		input.Config = map[string]string{}
	}
	manifest, ok := connectors.Find(api.Config.Connectors, input.ConnectorID)
	if !ok {
		writeError(writer, http.StatusNotFound, "not_found", "connector is not registered")
		return
	}
	if manifest.Auth == connectors.AuthOAuth2 || manifest.Auth == connectors.AuthAppInstall ||
		manifest.Auth == connectors.AuthUserSession ||
		manifest.ID == "webhook" {
		writeError(writer, http.StatusConflict, "managed_flow_required",
			"use the connector authorization or advanced webhook flow")
		return
	}
	service := api.connectorService()
	ctx, cancel := context.WithTimeout(request.Context(), api.Config.ProviderTimeout)
	defer cancel()
	result, err := service.Test(ctx, manifest, input.Config, input.Credentials)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "connector_verification_failed", err.Error())
		return
	}
	credentials := connectors.Credentials{Values: input.Credentials}
	if credentials.Values == nil {
		credentials.Values = map[string]string{}
	}
	cipher, err := api.encryptConnectorCredentials(credentials)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	item, err := api.Store.CreateConnectorConnection(
		request.Context(), input, "connected", result.AccountLabel, cipher, nil,
	)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	status, activationErr := service.Activate(ctx, item, credentials)
	lastError := ""
	if activationErr != nil {
		lastError = activationErr.Error()
	}
	if status != item.Status || lastError != "" {
		if err := api.Store.UpdateConnectorState(
			request.Context(), item.ID, status, result.AccountLabel, lastError, true,
		); err != nil {
			api.storeError(writer, err)
			return
		}
		item.Status, item.LastError = status, lastError
	}
	writeJSON(writer, http.StatusCreated, map[string]any{
		"data": item, "test": result, "credentials_stored": true,
	})
}

func (api *API) testConnectorConnection(writer http.ResponseWriter, request *http.Request) {
	item, credentials, err := api.connectorConnection(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		api.storeError(writer, err)
		return
	}
	manifest, ok := connectors.Find(api.Config.Connectors, item.ConnectorID)
	if !ok {
		writeError(writer, http.StatusConflict, "connector_missing", "connector manifest is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), api.Config.ProviderTimeout)
	defer cancel()
	if item.ConnectorID == "telegram" {
		result, updatedCredentials, telegramErr := api.testTelegramAccount(ctx, credentials)
		if telegramErr != nil {
			_ = api.Store.UpdateConnectorState(
				request.Context(), item.ID, "error", "", telegramErr.Error(), true,
			)
			writeError(writer, http.StatusBadGateway, "connector_test_failed", telegramErr.Error())
			return
		}
		cipher, encryptErr := api.encryptConnectorCredentials(updatedCredentials)
		if encryptErr != nil {
			api.storeError(writer, encryptErr)
			return
		}
		if err := api.Store.UpdateConnectorCredentials(
			request.Context(), item.ID, cipher, nil,
		); err != nil {
			api.storeError(writer, err)
			return
		}
		if err := api.Store.UpdateConnectorState(
			request.Context(), item.ID, "connected", result.AccountLabel, "", true,
		); err != nil {
			api.storeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{
			"data": result, "status": "connected",
		})
		return
	}
	if item.ConnectorID == "github" {
		result, githubErr := api.testGitHubConnection(ctx, item)
		if githubErr != nil {
			_ = api.Store.UpdateConnectorState(
				request.Context(), item.ID, "error", "", githubErr.Error(), true,
			)
			writeError(writer, http.StatusBadGateway, "connector_test_failed", githubErr.Error())
			return
		}
		if err := api.Store.UpdateConnectorState(
			request.Context(), item.ID, "connected", result.AccountLabel, "", true,
		); err != nil {
			api.storeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{
			"data": result, "status": "connected",
		})
		return
	}
	if provider, oauth := connectors.OAuthSpec(item.ConnectorID, api.Config.Connectors); oauth {
		result, updatedCredentials, oauthErr := api.testConnectorOAuth(
			ctx, provider, credentials,
		)
		if oauthErr != nil {
			_ = api.Store.UpdateConnectorState(
				request.Context(), item.ID, "error", "", oauthErr.Error(), true,
			)
			writeError(writer, http.StatusBadGateway, "connector_test_failed", oauthErr.Error())
			return
		}
		if updatedCredentials.AccessToken != credentials.AccessToken ||
			updatedCredentials.ExpiresAt != credentials.ExpiresAt {
			cipher, encryptErr := api.encryptConnectorCredentials(updatedCredentials)
			if encryptErr != nil {
				api.storeError(writer, encryptErr)
				return
			}
			if storeErr := api.Store.UpdateConnectorCredentials(
				request.Context(), item.ID, cipher, updatedCredentials.ExpiresAt,
			); storeErr != nil {
				api.storeError(writer, storeErr)
				return
			}
		}
		status := "connected"
		activationError := ""
		if err := api.Store.UpdateConnectorState(
			request.Context(), item.ID, status,
			result.AccountLabel, activationError, true,
		); err != nil {
			api.storeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{
			"data": result, "status": status,
			"activation_error": activationError,
		})
		return
	}
	result, err := api.connectorService().Test(ctx, manifest, item.Config, credentials.Values)
	if err != nil {
		_ = api.Store.UpdateConnectorState(
			request.Context(), item.ID, "error", "", err.Error(), true,
		)
		writeError(writer, http.StatusBadGateway, "connector_test_failed", err.Error())
		return
	}
	status, activationErr := api.connectorService().Activate(ctx, item, credentials)
	lastError := ""
	if activationErr != nil {
		lastError = activationErr.Error()
	}
	if err := api.Store.UpdateConnectorState(
		request.Context(), item.ID, status, result.AccountLabel, lastError, true,
	); err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": result, "status": status, "activation_error": lastError,
	})
}

func (api *API) connectorSample(writer http.ResponseWriter, request *http.Request) {
	item, _, err := api.connectorConnection(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		api.storeError(writer, err)
		return
	}
	event := connectors.NormalizedEvent{
		ExternalID: fmt.Sprintf("sample-%d", time.Now().UnixNano()),
		EventType:  "connector.sample",
		Subject:    item.Name + " test",
		Body:       "The connector reached Dispatch and entered the normal delivery pipeline.",
		Metadata: map[string]any{
			"connector": item.ConnectorID, "connection_id": item.ID, "sample": true,
		},
	}
	notificationID, created, err := api.enqueueConnectorEvent(request.Context(), item, event)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]any{
		"data": map[string]any{"notification_id": notificationID, "created": created},
	})
}

func (api *API) updateConnectorConnection(writer http.ResponseWriter, request *http.Request) {
	var input connectors.UpdateInput
	if !decode(writer, request, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.RecipientID = strings.TrimSpace(input.RecipientID)
	if input.Name == "" || input.RecipientID == "" {
		writeError(writer, http.StatusBadRequest, "validation_error", "name and recipient_id are required")
		return
	}
	item, _, err := api.connectorConnection(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		api.storeError(writer, err)
		return
	}
	if _, err := api.Store.GetRecipient(request.Context(), input.RecipientID); err != nil {
		api.storeError(writer, err)
		return
	}
	config, err := editableConnectorConfig(item.ConnectorID, item.Config, input.Config)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if item.ConnectorID == "google" && config["modules"] != item.Config["modules"] {
		writeError(writer, http.StatusConflict, "reauthorization_required",
			"Changing Google services requires authorization again")
		return
	}
	if err := api.Store.UpdateConnectorConnection(
		request.Context(), item.ID, input.Name, input.RecipientID, config,
	); err != nil {
		api.storeError(writer, err)
		return
	}
	item.Name, item.RecipientID, item.Config = input.Name, input.RecipientID, config
	writeJSON(writer, http.StatusOK, map[string]any{"data": item})
}

func (api *API) reauthorizeConnectorConnection(writer http.ResponseWriter, request *http.Request) {
	var input connectors.UpdateInput
	if !decode(writer, request, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.RecipientID = strings.TrimSpace(input.RecipientID)
	if input.Name == "" || input.RecipientID == "" {
		writeError(writer, http.StatusBadRequest, "validation_error", "name and recipient_id are required")
		return
	}
	item, _, err := api.connectorConnection(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		api.storeError(writer, err)
		return
	}
	provider, ok := connectors.OAuthSpec(item.ConnectorID, api.Config.Connectors)
	if !ok {
		writeError(writer, http.StatusConflict, "reauthorization_unavailable",
			"this connector does not use OAuth reauthorization")
		return
	}
	if _, err := api.Store.GetRecipient(request.Context(), input.RecipientID); err != nil {
		api.storeError(writer, err)
		return
	}
	config, err := editableConnectorConfig(item.ConnectorID, item.Config, input.Config)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if item.ConnectorID == "google" {
		provider.Scopes = connectors.GoogleScopes(connectors.GoogleModules(config["modules"]))
	}
	state, err := connectors.RandomURLToken(32)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	verifier, err := connectors.RandomURLToken(48)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	verifierCipher, err := ingress.EncryptSecret(api.Config.Connectors.EncryptionKey, verifier)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	if err := api.Store.CreateOAuthState(request.Context(), connectors.OAuthState{
		StateHash: connectors.StateHash(state), ConnectionID: item.ID,
		ConnectorID: item.ConnectorID, ConnectionName: input.Name,
		RecipientID: input.RecipientID, Config: config, VerifierCipher: verifierCipher,
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}); err != nil {
		api.storeError(writer, err)
		return
	}
	redirectURI := api.Config.Connectors.PublicURL + "/connect/v1/oauth/" +
		url.PathEscape(item.ConnectorID) + "/callback"
	writeJSON(writer, http.StatusOK, map[string]any{
		"authorization_url": connectors.OAuthValues(provider, redirectURI, state, verifier),
	})
}

func editableConnectorConfig(
	connectorID string,
	current, requested map[string]string,
) (map[string]string, error) {
	result := make(map[string]string, len(current)+3)
	for key, value := range current {
		result[key] = value
	}
	switch connectorID {
	case "discord":
		mode := strings.TrimSpace(requested["mode"])
		switch mode {
		case connectors.DiscordModeDirectMessages, connectors.DiscordModeMentions, connectors.DiscordModeAll:
			result["mode"] = mode
		default:
			return nil, fmt.Errorf("Discord mode must be direct_messages, mentions, or all")
		}
	case "github":
		mode := strings.TrimSpace(requested["mode"])
		switch mode {
		case connectors.GitHubModeImportant, connectors.GitHubModeCode, connectors.GitHubModeWork,
			connectors.GitHubModeCI, connectors.GitHubModeAll:
			result["mode"] = mode
		default:
			return nil, fmt.Errorf("GitHub mode is invalid")
		}
	case "google":
		mode := strings.TrimSpace(requested["mode"])
		switch mode {
		case connectors.GmailModeInbox, connectors.GmailModeUnread, connectors.GmailModeImportant:
			result["mode"] = mode
		default:
			return nil, fmt.Errorf("Gmail mode must be inbox, unread, or important")
		}
		modules := strings.TrimSpace(requested["modules"])
		if !connectors.ValidGoogleModules(modules) {
			return nil, fmt.Errorf("select at least one supported Google module")
		}
		result["modules"] = connectors.GoogleModulesValue(connectors.GoogleModules(modules))
		reminder := strings.TrimSpace(requested["calendar_reminder_minutes"])
		switch reminder {
		case "", "5", "15", "30", "60":
		default:
			return nil, fmt.Errorf("Calendar reminder must be 5, 15, 30, or 60 minutes")
		}
		if reminder == "" {
			reminder = "15"
		}
		result["calendar_reminder_minutes"] = reminder
	}
	return result, nil
}

func (api *API) setConnectorEnabled(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(writer, request, &input) {
		return
	}
	item, credentials, err := api.connectorConnection(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		api.storeError(writer, err)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), api.Config.ProviderTimeout)
	defer cancel()
	if !input.Enabled {
		if err := api.connectorService().Deactivate(ctx, item, credentials); err != nil {
			_ = api.Store.UpdateConnectorState(
				request.Context(), item.ID, "error", "", err.Error(), false,
			)
			writeError(writer, http.StatusBadGateway, "connector_deactivation_failed", err.Error())
			return
		}
		if err := api.Store.UpdateConnectorState(
			request.Context(), item.ID, "disabled", "", "", false,
		); err != nil {
			api.storeError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if item.ConnectorID == "github" {
		result, githubErr := api.testGitHubConnection(ctx, item)
		if githubErr != nil {
			_ = api.Store.UpdateConnectorState(
				request.Context(), item.ID, "error", "", githubErr.Error(), true,
			)
			writeError(writer, http.StatusBadGateway, "connector_activation_failed", githubErr.Error())
			return
		}
		if err := api.Store.UpdateConnectorState(
			request.Context(), item.ID, "connected", result.AccountLabel, "", true,
		); err != nil {
			api.storeError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if provider, oauth := connectors.OAuthSpec(item.ConnectorID, api.Config.Connectors); oauth {
		result, updatedCredentials, oauthErr := api.testConnectorOAuth(ctx, provider, credentials)
		if oauthErr != nil {
			_ = api.Store.UpdateConnectorState(
				request.Context(), item.ID, "error", "", oauthErr.Error(), true,
			)
			writeError(writer, http.StatusBadGateway, "connector_activation_failed", oauthErr.Error())
			return
		}
		if updatedCredentials.AccessToken != credentials.AccessToken ||
			updatedCredentials.ExpiresAt != credentials.ExpiresAt {
			cipher, encryptErr := api.encryptConnectorCredentials(updatedCredentials)
			if encryptErr != nil {
				api.storeError(writer, encryptErr)
				return
			}
			if storeErr := api.Store.UpdateConnectorCredentials(
				request.Context(), item.ID, cipher, updatedCredentials.ExpiresAt,
			); storeErr != nil {
				api.storeError(writer, storeErr)
				return
			}
		}
		status := "connected"
		activationError := ""
		if err := api.Store.UpdateConnectorState(
			request.Context(), item.ID, status, result.AccountLabel,
			activationError, true,
		); err != nil {
			api.storeError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	status, activationErr := api.connectorService().Activate(ctx, item, credentials)
	lastError := ""
	if activationErr != nil {
		lastError = activationErr.Error()
	}
	if err := api.Store.UpdateConnectorState(
		request.Context(), item.ID, status, "", lastError, false,
	); err != nil {
		api.storeError(writer, err)
		return
	}
	if activationErr != nil {
		writeError(writer, http.StatusBadGateway, "connector_activation_failed", activationErr.Error())
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) deleteConnectorConnection(writer http.ResponseWriter, request *http.Request) {
	item, credentials, err := api.connectorConnection(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		api.storeError(writer, err)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), api.Config.ProviderTimeout)
	defer cancel()
	if provider, oauth := connectors.OAuthSpec(item.ConnectorID, api.Config.Connectors); oauth {
		if err := api.revokeConnectorOAuth(ctx, provider, credentials); err != nil {
			writeError(writer, http.StatusBadGateway, "oauth_revocation_failed", err.Error())
			return
		}
	}
	if err := api.connectorService().Deactivate(ctx, item, credentials); err != nil {
		writeError(writer, http.StatusBadGateway, "connector_deactivation_failed", err.Error())
		return
	}
	if err := api.Store.DeleteConnectorConnection(request.Context(), item.ID); err != nil {
		api.storeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) beginConnectorOAuth(writer http.ResponseWriter, request *http.Request) {
	connectorID := chi.URLParam(request, "connector")
	provider, ok := connectors.OAuthSpec(connectorID, api.Config.Connectors)
	if !ok {
		writeError(writer, http.StatusConflict, "oauth_not_configured",
			"OAuth credentials for this connector are not configured")
		return
	}
	var input connectors.OAuthStartInput
	if !decode(writer, request, &input) {
		return
	}
	input.Name, input.RecipientID = strings.TrimSpace(input.Name), strings.TrimSpace(input.RecipientID)
	if input.Name == "" || input.RecipientID == "" {
		writeError(writer, http.StatusBadRequest, "validation_error", "name and recipient_id are required")
		return
	}
	if input.Config == nil {
		input.Config = map[string]string{}
	}
	if connectorID == "discord" {
		mode := strings.TrimSpace(input.Config["mode"])
		switch mode {
		case connectors.DiscordModeDirectMessages, connectors.DiscordModeMentions,
			connectors.DiscordModeAll:
		default:
			writeError(writer, http.StatusBadRequest, "validation_error",
				"Discord mode must be direct_messages, mentions, or all")
			return
		}
		input.Config = map[string]string{"mode": mode}
	}
	if connectorID == "google" {
		mode := strings.TrimSpace(input.Config["mode"])
		switch mode {
		case connectors.GmailModeInbox, connectors.GmailModeUnread,
			connectors.GmailModeImportant:
		default:
			writeError(writer, http.StatusBadRequest, "validation_error",
				"Gmail mode must be inbox, unread, or important")
			return
		}
		modules := strings.TrimSpace(input.Config["modules"])
		if !connectors.ValidGoogleModules(modules) {
			writeError(writer, http.StatusBadRequest, "validation_error",
				"Select at least one supported Google module")
			return
		}
		reminderMinutes := strings.TrimSpace(input.Config["calendar_reminder_minutes"])
		switch reminderMinutes {
		case "", "5", "15", "30", "60":
		default:
			writeError(writer, http.StatusBadRequest, "validation_error",
				"Calendar reminder must be 5, 15, 30, or 60 minutes")
			return
		}
		if reminderMinutes == "" {
			reminderMinutes = "15"
		}
		input.Config = map[string]string{
			"mode":                      mode,
			"modules":                   connectors.GoogleModulesValue(connectors.GoogleModules(modules)),
			"calendar_reminder_minutes": reminderMinutes,
		}
		provider.Scopes = connectors.GoogleScopes(
			connectors.GoogleModules(input.Config["modules"]),
		)
	}
	state, err := connectors.RandomURLToken(32)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	verifier, err := connectors.RandomURLToken(48)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	verifierCipher, err := ingress.EncryptSecret(api.Config.Connectors.EncryptionKey, verifier)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	if err := api.Store.CreateOAuthState(request.Context(), connectors.OAuthState{
		StateHash: connectors.StateHash(state), ConnectorID: connectorID,
		ConnectionName: input.Name, RecipientID: input.RecipientID,
		Config: input.Config, VerifierCipher: verifierCipher,
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}); err != nil {
		api.storeError(writer, err)
		return
	}
	redirectURI := api.Config.Connectors.PublicURL + "/connect/v1/oauth/" +
		url.PathEscape(connectorID) + "/callback"
	writeJSON(writer, http.StatusOK, map[string]any{
		"authorization_url": connectors.OAuthValues(
			provider, redirectURI, state, verifier,
		),
	})
}

func (api *API) connectorOAuthCallback(writer http.ResponseWriter, request *http.Request) {
	connectorID := chi.URLParam(request, "connector")
	if providerError := request.URL.Query().Get("error"); providerError != "" {
		api.redirectConnectorResult(writer, request, connectorID, providerError)
		return
	}
	code, state := request.URL.Query().Get("code"), request.URL.Query().Get("state")
	if code == "" || state == "" {
		api.redirectConnectorResult(writer, request, connectorID, "invalid_callback")
		return
	}
	pending, err := api.Store.ConsumeOAuthState(request.Context(), connectors.StateHash(state))
	if err != nil || pending.ConnectorID != connectorID {
		api.redirectConnectorResult(writer, request, connectorID, "invalid_or_expired_state")
		return
	}
	provider, ok := connectors.OAuthSpec(connectorID, api.Config.Connectors)
	if !ok {
		api.redirectConnectorResult(writer, request, connectorID, "oauth_not_configured")
		return
	}
	verifier, err := ingress.DecryptSecret(
		api.Config.Connectors.EncryptionKey, pending.VerifierCipher,
	)
	if err != nil {
		api.redirectConnectorResult(writer, request, connectorID, "credential_decryption_failed")
		return
	}
	credentials, accountLabel, err := api.exchangeConnectorOAuth(
		request.Context(), provider, code, verifier,
	)
	if err != nil {
		api.Logger.Error("connector OAuth exchange", "connector", connectorID, "error", err)
		api.redirectConnectorResult(writer, request, connectorID, "oauth_exchange_failed")
		return
	}
	connectionConfig := pending.Config
	if connectionConfig == nil {
		connectionConfig = map[string]string{}
	}
	if connectorID == "discord" {
		connectionConfig["discord_user_id"] = credentials.Values["provider_user_id"]
		connectionConfig["guild_id"] = credentials.Values["guild_id"]
		connectionConfig["guild_name"] = credentials.Values["guild_name"]
		if connectionConfig["discord_user_id"] == "" || connectionConfig["guild_id"] == "" {
			api.redirectConnectorResult(writer, request, connectorID, "discord_installation_incomplete")
			return
		}
	}
	if (connectorID == "google" || connectorID == "youtube") &&
		credentials.RefreshToken == "" {
		if pending.ConnectionID == "" {
			api.redirectConnectorResult(writer, request, connectorID, connectorID+"_refresh_token_missing")
			return
		}
		existing, existingCredentials, existingErr := api.connectorConnection(
			request.Context(), pending.ConnectionID,
		)
		if existingErr != nil || existing.ConnectorID != connectorID ||
			existingCredentials.RefreshToken == "" {
			api.redirectConnectorResult(writer, request, connectorID, connectorID+"_refresh_token_missing")
			return
		}
		credentials.RefreshToken = existingCredentials.RefreshToken
	}
	cipher, err := api.encryptConnectorCredentials(credentials)
	if err != nil {
		api.redirectConnectorResult(writer, request, connectorID, "credential_encryption_failed")
		return
	}
	status := "connected"
	if pending.ConnectionID != "" {
		existing, _, existingErr := api.connectorConnection(request.Context(), pending.ConnectionID)
		if existingErr != nil || existing.ConnectorID != connectorID {
			api.redirectConnectorResult(writer, request, connectorID, "connection_update_failed")
			return
		}
		if err := api.Store.UpdateConnectorConnection(
			request.Context(), existing.ID, pending.ConnectionName,
			pending.RecipientID, connectionConfig,
		); err != nil {
			api.redirectConnectorResult(writer, request, connectorID, "connection_update_failed")
			return
		}
		if err := api.Store.UpdateConnectorCredentials(
			request.Context(), existing.ID, cipher, credentials.ExpiresAt,
		); err != nil {
			api.redirectConnectorResult(writer, request, connectorID, "connection_update_failed")
			return
		}
		if err := api.Store.UpdateConnectorState(
			request.Context(), existing.ID, status, accountLabel, "", true,
		); err != nil {
			api.redirectConnectorResult(writer, request, connectorID, "connection_update_failed")
			return
		}
		api.redirectConnectorResult(writer, request, connectorID, "")
		return
	}
	_, err = api.Store.CreateConnectorConnection(request.Context(), connectors.CreateInput{
		ConnectorID: connectorID, Name: pending.ConnectionName,
		RecipientID: pending.RecipientID, Config: connectionConfig,
	}, status, accountLabel, cipher, credentials.ExpiresAt)
	if err != nil {
		api.redirectConnectorResult(writer, request, connectorID, "connection_create_failed")
		return
	}
	api.redirectConnectorResult(writer, request, connectorID, "")
}

func (api *API) enqueueConnectorEvent(
	ctx context.Context,
	connection connectors.Connection,
	event connectors.NormalizedEvent,
) (string, bool, error) {
	idempotencyDigest := sha256.Sum256(
		[]byte(connection.ID + ":" + event.ExternalID),
	)
	item := notification.Notification{
		IdempotencyKey: "con_" + hex.EncodeToString(idempotencyDigest[:]),
		RecipientID:    connection.RecipientID, EventType: event.EventType,
		Subject: event.Subject, Body: event.Body,
		Metadata: connectors.ConnectionMetadata(connection, event.Metadata),
	}
	created, err := api.Store.CreateNotification(ctx, &item)
	if err != nil {
		return "", false, err
	}
	if err := api.Store.RecordConnectorEvent(
		ctx, connection.ID, event.ExternalID, item.ID, event.EventType,
	); err != nil {
		return "", false, err
	}
	if err := api.Store.TouchConnectorEvent(ctx, connection.ID); err != nil {
		return "", false, err
	}
	return item.ID, created, nil
}

func (api *API) connectorConnection(
	ctx context.Context,
	id string,
) (connectors.Connection, connectors.Credentials, error) {
	item, cipher, err := api.Store.GetConnectorConnection(ctx, id)
	if err != nil {
		return item, connectors.Credentials{}, err
	}
	credentials := connectors.Credentials{Values: map[string]string{}}
	if len(cipher) == 0 {
		return item, credentials, nil
	}
	raw, err := ingress.DecryptSecret(api.Config.Connectors.EncryptionKey, cipher)
	if err != nil {
		return item, credentials, err
	}
	err = json.Unmarshal([]byte(raw), &credentials)
	return item, credentials, err
}

func (api *API) encryptConnectorCredentials(credentials connectors.Credentials) ([]byte, error) {
	raw, err := json.Marshal(credentials)
	if err != nil {
		return nil, err
	}
	return ingress.EncryptSecret(api.Config.Connectors.EncryptionKey, string(raw))
}

func (api *API) connectorService() connectors.Service {
	return connectors.Service{PublicURL: api.Config.Connectors.PublicURL}
}

func (api *API) exchangeConnectorOAuth(
	ctx context.Context,
	provider connectors.OAuthProvider,
	code, verifier string,
) (connectors.Credentials, string, error) {
	redirectURI := api.Config.Connectors.PublicURL + "/connect/v1/oauth/" +
		url.PathEscape(provider.ID) + "/callback"
	values := url.Values{
		"client_id": {provider.ClientID}, "client_secret": {provider.ClientSecret},
		"code": {code}, "code_verifier": {verifier},
		"grant_type": {"authorization_code"}, "redirect_uri": {redirectURI},
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, provider.TokenURL, strings.NewReader(values.Encode()),
	)
	if err != nil {
		return connectors.Credentials{}, "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: api.Config.ProviderTimeout}
	response, err := client.Do(request)
	if err != nil {
		return connectors.Credentials{}, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return connectors.Credentials{}, "", fmt.Errorf("token endpoint returned %s", response.Status)
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int64  `json:"expires_in"`
		Guild        *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"guild"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return connectors.Credentials{}, "", err
	}
	if token.AccessToken == "" {
		return connectors.Credentials{}, "", fmt.Errorf("token endpoint omitted access_token")
	}
	var expiresAt *time.Time
	if token.ExpiresIn > 0 {
		value := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
		expiresAt = &value
	}
	valuesMap := map[string]string{}
	if token.Guild != nil {
		valuesMap["guild_id"] = token.Guild.ID
		valuesMap["guild_name"] = token.Guild.Name
	}
	accountLabel := provider.ID + " account"
	if provider.UserInfoURL != "" {
		userRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.UserInfoURL, nil)
		if err == nil {
			userRequest.Header.Set("Authorization", "Bearer "+token.AccessToken)
			if userResponse, userErr := client.Do(userRequest); userErr == nil {
				defer userResponse.Body.Close()
				var profile struct {
					ID         string `json:"id"`
					Sub        string `json:"sub"`
					Email      string `json:"email"`
					Name       string `json:"name"`
					Username   string `json:"username"`
					GlobalName string `json:"global_name"`
				}
				if userResponse.StatusCode >= 200 && userResponse.StatusCode < 300 &&
					json.NewDecoder(io.LimitReader(userResponse.Body, 1<<20)).Decode(&profile) == nil {
					valuesMap["provider_user_id"] = profile.ID
					if valuesMap["provider_user_id"] == "" {
						valuesMap["provider_user_id"] = profile.Sub
					}
					valuesMap["provider_username"] = profile.Username
					if profile.GlobalName != "" {
						accountLabel = profile.GlobalName
					} else if profile.Username != "" {
						accountLabel = profile.Username
					} else if profile.Email != "" {
						accountLabel = profile.Email
					} else if profile.Name != "" {
						accountLabel = profile.Name
					}
				}
			}
		}
	}
	return connectors.Credentials{
		Values: valuesMap, AccessToken: token.AccessToken,
		RefreshToken: token.RefreshToken, TokenType: token.TokenType,
		Scope: token.Scope, ExpiresAt: expiresAt,
	}, accountLabel, nil
}

func (api *API) testConnectorOAuth(
	ctx context.Context,
	provider connectors.OAuthProvider,
	credentials connectors.Credentials,
) (connectors.TestResult, connectors.Credentials, error) {
	if credentials.AccessToken == "" {
		return connectors.TestResult{}, credentials, fmt.Errorf("OAuth access token is missing")
	}
	if credentials.ExpiresAt != nil && time.Until(*credentials.ExpiresAt) < 30*time.Second {
		if credentials.RefreshToken == "" {
			return connectors.TestResult{}, credentials, fmt.Errorf("OAuth token expired and no refresh token is available")
		}
		values := url.Values{
			"client_id": {provider.ClientID}, "client_secret": {provider.ClientSecret},
			"refresh_token": {credentials.RefreshToken}, "grant_type": {"refresh_token"},
		}
		request, err := http.NewRequestWithContext(
			ctx, http.MethodPost, provider.TokenURL, strings.NewReader(values.Encode()),
		)
		if err != nil {
			return connectors.TestResult{}, credentials, err
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := (&http.Client{Timeout: api.Config.ProviderTimeout}).Do(request)
		if err != nil {
			return connectors.TestResult{}, credentials, err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return connectors.TestResult{}, credentials, fmt.Errorf("OAuth refresh returned %s", response.Status)
		}
		var refreshed struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			Scope       string `json:"scope"`
			ExpiresIn   int64  `json:"expires_in"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&refreshed); err != nil {
			return connectors.TestResult{}, credentials, err
		}
		if refreshed.AccessToken == "" {
			return connectors.TestResult{}, credentials, fmt.Errorf("OAuth refresh omitted access_token")
		}
		credentials.AccessToken = refreshed.AccessToken
		if refreshed.TokenType != "" {
			credentials.TokenType = refreshed.TokenType
		}
		if refreshed.Scope != "" {
			credentials.Scope = refreshed.Scope
		}
		if refreshed.ExpiresIn > 0 {
			expiresAt := time.Now().UTC().Add(time.Duration(refreshed.ExpiresIn) * time.Second)
			credentials.ExpiresAt = &expiresAt
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.UserInfoURL, nil)
	if err != nil {
		return connectors.TestResult{}, credentials, err
	}
	request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
	response, err := (&http.Client{Timeout: api.Config.ProviderTimeout}).Do(request)
	if err != nil {
		return connectors.TestResult{}, credentials, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return connectors.TestResult{}, credentials, fmt.Errorf("provider identity check returned %s", response.Status)
	}
	var profile struct {
		Email      string `json:"email"`
		Name       string `json:"name"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile); err != nil {
		return connectors.TestResult{}, credentials, err
	}
	label := profile.GlobalName
	if label == "" {
		label = profile.Username
	}
	if label == "" {
		label = profile.Email
	}
	if label == "" {
		label = profile.Name
	}
	return connectors.TestResult{
		OK: true, AccountLabel: label, Message: "OAuth credentials verified.",
	}, credentials, nil
}

func (api *API) revokeConnectorOAuth(
	ctx context.Context,
	provider connectors.OAuthProvider,
	credentials connectors.Credentials,
) error {
	if provider.RevokeURL == "" {
		return nil
	}
	token := credentials.RefreshToken
	if token == "" {
		token = credentials.AccessToken
	}
	if token == "" {
		return fmt.Errorf("OAuth token is missing")
	}
	values := url.Values{"token": {token}}
	if provider.ID == "discord" {
		values.Set("client_id", provider.ClientID)
		values.Set("client_secret", provider.ClientSecret)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, provider.RevokeURL, strings.NewReader(values.Encode()),
	)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := (&http.Client{Timeout: api.Config.ProviderTimeout}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("OAuth revocation returned %s", response.Status)
	}
	return nil
}

func (api *API) redirectConnectorResult(
	writer http.ResponseWriter,
	request *http.Request,
	connectorID, errorCode string,
) {
	target, err := url.Parse(api.Config.Connectors.PublicURL + "/integrations")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "redirect_error", "invalid connector public URL")
		return
	}
	query := target.Query()
	if errorCode == "" {
		query.Set("connected", connectorID)
	} else {
		query.Set("integration_error", errorCode)
	}
	target.RawQuery = query.Encode()
	http.Redirect(writer, request, target.String(), http.StatusSeeOther)
}
