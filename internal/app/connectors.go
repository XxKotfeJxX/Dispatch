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
	if manifest.ID == "youtube" {
		secret, err := ingress.GenerateSecret()
		if err != nil {
			api.storeError(writer, err)
			return
		}
		credentials.Values["hook_secret"] = secret
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
		if err := api.Store.UpdateConnectorState(
			request.Context(), item.ID, "action_required",
			result.AccountLabel, "", true,
		); err != nil {
			api.storeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{
			"data": result, "status": "action_required",
			"activation_error": "Gmail Pub/Sub topic and mailbox watch provisioning are required.",
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
		if err := api.Store.UpdateConnectorState(
			request.Context(), item.ID, "action_required", result.AccountLabel,
			"Gmail Pub/Sub watch provisioning is not configured yet.", true,
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
		VerifierCipher: verifierCipher, ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
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
	cipher, err := api.encryptConnectorCredentials(credentials)
	if err != nil {
		api.redirectConnectorResult(writer, request, connectorID, "credential_encryption_failed")
		return
	}
	_, err = api.Store.CreateConnectorConnection(request.Context(), connectors.CreateInput{
		ConnectorID: connectorID, Name: pending.ConnectionName,
		RecipientID: pending.RecipientID, Config: map[string]string{},
	}, "action_required", accountLabel, cipher, credentials.ExpiresAt)
	if err != nil {
		api.redirectConnectorResult(writer, request, connectorID, "connection_create_failed")
		return
	}
	api.redirectConnectorResult(writer, request, connectorID, "")
}

func (api *API) connectorWebhook(writer http.ResponseWriter, request *http.Request) {
	item, credentials, err := api.connectorConnection(request.Context(), chi.URLParam(request, "id"))
	if err != nil || !item.Enabled {
		writeError(writer, http.StatusNotFound, "not_found", "connector callback is unavailable")
		return
	}
	if request.Method == http.MethodGet && item.ConnectorID == "youtube" {
		if !secureConnectorValue(
			request.URL.Query().Get("token"), credentials.Values["hook_secret"],
		) {
			writeError(writer, http.StatusUnauthorized, "invalid_callback_token", "callback token is invalid")
			return
		}
		challenge := request.URL.Query().Get("hub.challenge")
		if challenge == "" {
			writeError(writer, http.StatusBadRequest, "invalid_challenge", "hub.challenge is required")
			return
		}
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(challenge))
		return
	}
	raw, err := io.ReadAll(request.Body)
	if err != nil || len(raw) == 0 {
		writeError(writer, http.StatusBadRequest, "invalid_body", "callback body is empty")
		return
	}
	if item.ConnectorID == "youtube" && !secureConnectorValue(
		request.URL.Query().Get("token"), credentials.Values["hook_secret"],
	) {
		writeError(writer, http.StatusUnauthorized, "invalid_callback_token", "callback token is invalid")
		return
	}
	event, err := connectors.VerifyAndNormalize(item, credentials, request.Header, raw)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "invalid_connector_event", err.Error())
		return
	}
	notificationID, created, err := api.enqueueConnectorEvent(request.Context(), item, event)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": map[string]any{"notification_id": notificationID, "created": created},
	})
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
		Subject: event.Subject, Body: event.Body, Metadata: event.Metadata,
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
	accountLabel := provider.ID + " account"
	if provider.UserInfoURL != "" {
		userRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.UserInfoURL, nil)
		if err == nil {
			userRequest.Header.Set("Authorization", "Bearer "+token.AccessToken)
			if userResponse, userErr := client.Do(userRequest); userErr == nil {
				defer userResponse.Body.Close()
				var profile struct {
					Email string `json:"email"`
					Name  string `json:"name"`
				}
				if userResponse.StatusCode >= 200 && userResponse.StatusCode < 300 &&
					json.NewDecoder(io.LimitReader(userResponse.Body, 1<<20)).Decode(&profile) == nil {
					if profile.Email != "" {
						accountLabel = profile.Email
					} else if profile.Name != "" {
						accountLabel = profile.Name
					}
				}
			}
		}
	}
	return connectors.Credentials{
		Values: map[string]string{}, AccessToken: token.AccessToken,
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
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile); err != nil {
		return connectors.TestResult{}, credentials, err
	}
	label := profile.Email
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

func secureConnectorValue(provided, expected string) bool {
	return expected != "" && provided != "" &&
		ingress.VerifyStoredHash(ingress.HashSecret(expected), provided)
}
