package app

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dispatch/internal/connectors"
)

type githubInstallation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
	} `json:"account"`
	TargetType          string `json:"target_type"`
	RepositorySelection string `json:"repository_selection"`
}

func (api *API) beginGitHubInstall(writer http.ResponseWriter, request *http.Request) {
	if !api.githubConfigured() {
		writeError(writer, http.StatusConflict, "github_not_configured",
			"GitHub App credentials are not configured")
		return
	}
	var input connectors.OAuthStartInput
	if !decode(writer, request, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.RecipientID = strings.TrimSpace(input.RecipientID)
	if input.Name == "" || input.RecipientID == "" {
		writeError(writer, http.StatusBadRequest, "validation_error",
			"name and recipient_id are required")
		return
	}
	mode := strings.TrimSpace(input.Config["mode"])
	switch mode {
	case connectors.GitHubModeImportant, connectors.GitHubModeCode,
		connectors.GitHubModeWork, connectors.GitHubModeCI, connectors.GitHubModeAll:
	default:
		writeError(writer, http.StatusBadRequest, "validation_error",
			"GitHub mode must be important, code, work, ci, or all")
		return
	}
	state, err := connectors.RandomURLToken(32)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	if err := api.Store.CreateOAuthState(request.Context(), connectors.OAuthState{
		StateHash:      connectors.StateHash(state),
		ConnectorID:    "github",
		ConnectionName: input.Name,
		RecipientID:    input.RecipientID,
		Config:         map[string]string{"mode": mode},
		VerifierCipher: []byte{},
		ExpiresAt:      time.Now().UTC().Add(15 * time.Minute),
	}); err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{
		"authorization_url": connectors.GitHubInstallURL(
			api.Config.Connectors.GitHubAppSlug, state,
		),
	})
}

func (api *API) completeGitHubInstall(writer http.ResponseWriter, request *http.Request) {
	state := strings.TrimSpace(request.URL.Query().Get("state"))
	if state == "" {
		api.redirectConnectorResult(writer, request, "github",
			"github_installation_must_start_in_dispatch")
		return
	}
	installationIDRaw := strings.TrimSpace(request.URL.Query().Get("installation_id"))
	if installationIDRaw == "" {
		api.redirectConnectorResult(writer, request, "github",
			"github_installation_id_missing")
		return
	}
	installationID, parseErr := strconv.ParseInt(installationIDRaw, 10, 64)
	if parseErr != nil || installationID <= 0 {
		api.redirectConnectorResult(writer, request, "github",
			"github_installation_id_invalid")
		return
	}
	pending, err := api.Store.ConsumeOAuthState(
		request.Context(), connectors.StateHash(state),
	)
	if err != nil || pending.ConnectorID != "github" {
		api.redirectConnectorResult(writer, request, "github", "invalid_or_expired_state")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), api.Config.ProviderTimeout)
	defer cancel()
	installation, err := api.getGitHubInstallation(ctx, installationID)
	if err != nil {
		api.Logger.Error("verify GitHub App installation",
			"installation_id", installationID, "error", err)
		api.redirectConnectorResult(writer, request, "github", "installation_verification_failed")
		return
	}
	connectionConfig := pending.Config
	if connectionConfig == nil {
		connectionConfig = map[string]string{"mode": connectors.GitHubModeImportant}
	}
	connectionConfig["installation_id"] = strconv.FormatInt(installation.ID, 10)
	connectionConfig["account"] = installation.Account.Login
	connectionConfig["target_type"] = installation.TargetType
	connectionConfig["repository_selection"] = installation.RepositorySelection
	_, err = api.Store.CreateConnectorConnection(
		request.Context(),
		connectors.CreateInput{
			ConnectorID: "github",
			Name:        pending.ConnectionName,
			RecipientID: pending.RecipientID,
			Config:      connectionConfig,
		},
		"connected", installation.Account.Login, nil, nil,
	)
	if err != nil {
		api.redirectConnectorResult(writer, request, "github", "connection_create_failed")
		return
	}
	api.redirectConnectorResult(writer, request, "github", "")
}

func (api *API) githubWebhook(writer http.ResponseWriter, request *http.Request) {
	raw, err := io.ReadAll(request.Body)
	if err != nil || len(raw) == 0 {
		writeError(writer, http.StatusBadRequest, "invalid_body", "GitHub webhook body is empty")
		return
	}
	if !connectors.VerifyGitHubSignature(
		api.Config.Connectors.GitHubWebhookSecret,
		request.Header.Get("X-Hub-Signature-256"),
		raw,
	) {
		writeError(writer, http.StatusUnauthorized, "invalid_signature",
			"GitHub webhook signature is invalid")
		return
	}
	event := strings.TrimSpace(request.Header.Get("X-GitHub-Event"))
	delivery := strings.TrimSpace(request.Header.Get("X-GitHub-Delivery"))
	if event == "ping" {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "pong"})
		return
	}
	if event == "" || delivery == "" {
		writeError(writer, http.StatusBadRequest, "invalid_headers",
			"X-GitHub-Event and X-GitHub-Delivery are required")
		return
	}
	payloadBody := raw
	if strings.HasPrefix(
		strings.ToLower(request.Header.Get("Content-Type")),
		"application/x-www-form-urlencoded",
	) {
		values, formErr := url.ParseQuery(string(raw))
		if formErr != nil || values.Get("payload") == "" {
			writeError(writer, http.StatusBadRequest, "invalid_payload",
				"GitHub form payload is invalid")
			return
		}
		payloadBody = []byte(values.Get("payload"))
	}
	payload, err := connectors.DecodeGitHubPayload(payloadBody)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	connections, err := api.Store.ListConnectorConnections(request.Context())
	if err != nil {
		api.storeError(writer, err)
		return
	}
	normalized := connectors.NormalizeGitHubEvent(event, delivery, payload)
	accepted := 0
	notificationIDs := make([]string, 0)
	for _, connection := range connections {
		if !connectors.GitHubEventMatches(connection, event, payload) {
			continue
		}
		notificationID, created, enqueueErr := api.enqueueConnectorEvent(
			request.Context(), connection, normalized,
		)
		if enqueueErr != nil {
			api.storeError(writer, enqueueErr)
			return
		}
		if created {
			accepted++
			notificationIDs = append(notificationIDs, notificationID)
		}
	}
	writeJSON(writer, http.StatusAccepted, map[string]any{
		"accepted": accepted, "notification_ids": notificationIDs,
	})
}

func (api *API) githubConfigured() bool {
	cfg := api.Config.Connectors
	return cfg.GitHubAppSlug != "" && cfg.GitHubAppID > 0 &&
		cfg.GitHubWebhookSecret != "" && cfg.GitHubPrivateKeyB64 != ""
}

func (api *API) testGitHubConnection(
	ctx context.Context,
	connection connectors.Connection,
) (connectors.TestResult, error) {
	installationID, err := strconv.ParseInt(connection.Config["installation_id"], 10, 64)
	if err != nil || installationID <= 0 {
		return connectors.TestResult{}, errors.New("GitHub installation ID is missing")
	}
	installation, err := api.getGitHubInstallation(ctx, installationID)
	if err != nil {
		return connectors.TestResult{}, err
	}
	return connectors.TestResult{
		OK:           true,
		AccountLabel: installation.Account.Login,
		Message:      "GitHub App installation verified.",
	}, nil
}

func (api *API) getGitHubInstallation(
	ctx context.Context,
	installationID int64,
) (githubInstallation, error) {
	var installation githubInstallation
	token, err := api.githubAppJWT(time.Now().UTC())
	if err != nil {
		return installation, err
	}
	endpoint := "https://api.github.com/app/installations/" +
		strconv.FormatInt(installationID, 10)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return installation, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "Dispatch")
	response, err := (&http.Client{Timeout: api.Config.ProviderTimeout}).Do(request)
	if err != nil {
		return installation, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return installation, fmt.Errorf("GitHub installation lookup returned %s", response.Status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&installation); err != nil {
		return installation, err
	}
	if installation.ID != installationID || installation.Account.Login == "" {
		return installation, errors.New("GitHub returned an incomplete installation")
	}
	return installation, nil
}

func (api *API) githubAppJWT(now time.Time) (string, error) {
	privateKey, err := decodeGitHubPrivateKey(api.Config.Connectors.GitHubPrivateKeyB64)
	if err != nil {
		return "", err
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]int64{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": api.Config.Connectors.GitHubAppID,
	})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func decodeGitHubPrivateKey(encoded string) (*rsa.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(encoded))
	}
	if err != nil {
		return nil, errors.New("GITHUB_PRIVATE_KEY_BASE64 is not valid base64")
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("GitHub private key does not contain a PEM block")
	}
	if key, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes); parseErr == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("GitHub private key is not a supported RSA key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub private key is not RSA")
	}
	return key, nil
}
