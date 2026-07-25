package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	telegramauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	"dispatch/internal/connectors"
)

var telegramPhonePattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

type telegramMemorySession struct {
	mu      sync.RWMutex
	data    []byte
	onStore func([]byte) error
}

func (storage *telegramMemorySession) LoadSession(context.Context) ([]byte, error) {
	storage.mu.RLock()
	defer storage.mu.RUnlock()
	if len(storage.data) == 0 {
		return nil, session.ErrNotFound
	}
	return append([]byte(nil), storage.data...), nil
}

func (storage *telegramMemorySession) StoreSession(_ context.Context, data []byte) error {
	copyOfData := append([]byte(nil), data...)
	storage.mu.Lock()
	storage.data = copyOfData
	storage.mu.Unlock()
	if storage.onStore != nil {
		return storage.onStore(copyOfData)
	}
	return nil
}

func (storage *telegramMemorySession) Bytes() []byte {
	storage.mu.RLock()
	defer storage.mu.RUnlock()
	return append([]byte(nil), storage.data...)
}

type telegramAuthAttempt struct {
	ID             string
	Phone          string
	CodeHash       string
	ConnectionName string
	RecipientID    string
	Session        *telegramMemorySession
	ExpiresAt      time.Time
}

var pendingTelegramAuth = struct {
	sync.Mutex
	values map[string]*telegramAuthAttempt
}{values: map[string]*telegramAuthAttempt{}}

type telegramAuthResponse struct {
	AuthID  string                 `json:"auth_id,omitempty"`
	Step    string                 `json:"step"`
	Message string                 `json:"message"`
	Data    *connectors.Connection `json:"data,omitempty"`
}

func (api *API) beginTelegramAccountAuth(writer http.ResponseWriter, request *http.Request) {
	if api.Config.Connectors.TelegramAPIID <= 0 || api.Config.Connectors.TelegramAPIHash == "" {
		writeError(writer, http.StatusConflict, "telegram_not_configured",
			"Telegram account connections are not enabled on this installation")
		return
	}
	var input struct {
		Name        string `json:"name"`
		RecipientID string `json:"recipient_id"`
		Phone       string `json:"phone_number"`
	}
	if !decode(writer, request, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.RecipientID = strings.TrimSpace(input.RecipientID)
	input.Phone = strings.ReplaceAll(strings.TrimSpace(input.Phone), " ", "")
	if input.Name == "" || input.RecipientID == "" || !telegramPhonePattern.MatchString(input.Phone) {
		writeError(writer, http.StatusBadRequest, "validation_error",
			"name, recipient_id, and a phone number in international format are required")
		return
	}
	if _, err := api.Store.GetRecipient(request.Context(), input.RecipientID); err != nil {
		api.storeError(writer, err)
		return
	}
	authID, err := connectors.RandomURLToken(24)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	attempt := &telegramAuthAttempt{
		ID: authID, Phone: input.Phone, ConnectionName: input.Name,
		RecipientID: input.RecipientID, Session: &telegramMemorySession{},
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}
	ctx, cancel := context.WithTimeout(request.Context(), telegramAuthTimeout(api))
	defer cancel()
	var sent tg.AuthSentCodeClass
	err = runTelegramClient(ctx, api.Config.Connectors.TelegramAPIID,
		api.Config.Connectors.TelegramAPIHash, attempt.Session, func(ctx context.Context, client *telegram.Client) error {
			var sendErr error
			sent, sendErr = client.Auth().SendCode(ctx, attempt.Phone, telegramauth.SendCodeOptions{})
			return sendErr
		})
	if err != nil {
		writeError(writer, http.StatusBadGateway, "telegram_authorization_failed",
			"Telegram could not send a sign-in code: "+err.Error())
		return
	}
	code, ok := sent.(*tg.AuthSentCode)
	if !ok || code.PhoneCodeHash == "" {
		writeError(writer, http.StatusBadGateway, "telegram_authorization_failed",
			"Telegram returned an unsupported authorization method")
		return
	}
	attempt.CodeHash = code.PhoneCodeHash
	pendingTelegramAuth.Lock()
	pendingTelegramAuth.values[authID] = attempt
	pendingTelegramAuth.Unlock()
	writeJSON(writer, http.StatusOK, telegramAuthResponse{
		AuthID: authID, Step: "code",
		Message: "Enter the code Telegram sent to your existing session or phone.",
	})
}

func (api *API) completeTelegramAccountCode(writer http.ResponseWriter, request *http.Request) {
	attempt, ok := telegramAttempt(chi.URLParam(request, "authID"))
	if !ok {
		writeError(writer, http.StatusNotFound, "telegram_auth_expired",
			"Telegram authorization expired; start again")
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if !decode(writer, request, &input) {
		return
	}
	input.Code = strings.TrimSpace(input.Code)
	if input.Code == "" {
		writeError(writer, http.StatusBadRequest, "validation_error", "Telegram code is required")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), telegramAuthTimeout(api))
	defer cancel()
	var authorization *tg.AuthAuthorization
	err := runTelegramClient(ctx, api.Config.Connectors.TelegramAPIID,
		api.Config.Connectors.TelegramAPIHash, attempt.Session, func(ctx context.Context, client *telegram.Client) error {
			var signInErr error
			authorization, signInErr = client.Auth().SignIn(ctx, attempt.Phone, input.Code, attempt.CodeHash)
			return signInErr
		})
	if errors.Is(err, telegramauth.ErrPasswordAuthNeeded) {
		writeJSON(writer, http.StatusOK, telegramAuthResponse{
			AuthID: attempt.ID, Step: "password",
			Message: "This Telegram account uses two-step verification. Enter its password.",
		})
		return
	}
	if err != nil {
		writeError(writer, http.StatusBadGateway, "telegram_code_rejected",
			"Telegram rejected the sign-in code: "+err.Error())
		return
	}
	api.finishTelegramAccountAuth(writer, request, attempt, authorization)
}

func (api *API) completeTelegramAccountPassword(writer http.ResponseWriter, request *http.Request) {
	attempt, ok := telegramAttempt(chi.URLParam(request, "authID"))
	if !ok {
		writeError(writer, http.StatusNotFound, "telegram_auth_expired",
			"Telegram authorization expired; start again")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if !decode(writer, request, &input) {
		return
	}
	if input.Password == "" {
		writeError(writer, http.StatusBadRequest, "validation_error", "Telegram 2FA password is required")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), telegramAuthTimeout(api))
	defer cancel()
	var authorization *tg.AuthAuthorization
	err := runTelegramClient(ctx, api.Config.Connectors.TelegramAPIID,
		api.Config.Connectors.TelegramAPIHash, attempt.Session, func(ctx context.Context, client *telegram.Client) error {
			var passwordErr error
			authorization, passwordErr = client.Auth().Password(ctx, input.Password)
			return passwordErr
		})
	input.Password = ""
	if err != nil {
		writeError(writer, http.StatusBadGateway, "telegram_password_rejected",
			"Telegram rejected the two-step verification password: "+err.Error())
		return
	}
	api.finishTelegramAccountAuth(writer, request, attempt, authorization)
}

func (api *API) finishTelegramAccountAuth(
	writer http.ResponseWriter,
	request *http.Request,
	attempt *telegramAuthAttempt,
	authorization *tg.AuthAuthorization,
) {
	user, ok := authorization.User.(*tg.User)
	if !ok {
		writeError(writer, http.StatusBadGateway, "telegram_authorization_failed",
			"Telegram did not return the authorized account")
		return
	}
	credentials := connectors.Credentials{Values: map[string]string{
		"phone_number": attempt.Phone,
		"session":      base64.StdEncoding.EncodeToString(attempt.Session.Bytes()),
	}}
	cipher, err := api.encryptConnectorCredentials(credentials)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	item, err := api.Store.CreateConnectorConnection(request.Context(), connectors.CreateInput{
		ConnectorID: "telegram", Name: attempt.ConnectionName,
		RecipientID: attempt.RecipientID,
		Config:      map[string]string{"user_id": fmt.Sprint(user.ID)},
	}, "connected", telegramUserLabel(user), cipher, nil)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	pendingTelegramAuth.Lock()
	delete(pendingTelegramAuth.values, attempt.ID)
	pendingTelegramAuth.Unlock()
	writeJSON(writer, http.StatusCreated, telegramAuthResponse{
		Step: "connected", Message: "Telegram account connected.", Data: &item,
	})
}

func telegramAttempt(id string) (*telegramAuthAttempt, bool) {
	pendingTelegramAuth.Lock()
	defer pendingTelegramAuth.Unlock()
	attempt, ok := pendingTelegramAuth.values[id]
	if !ok {
		return nil, false
	}
	if time.Now().UTC().After(attempt.ExpiresAt) {
		delete(pendingTelegramAuth.values, id)
		return nil, false
	}
	return attempt, true
}

func runTelegramClient(
	ctx context.Context,
	appID int,
	appHash string,
	storage telegram.SessionStorage,
	run func(context.Context, *telegram.Client) error,
) error {
	client := telegram.NewClient(appID, appHash, telegram.Options{
		SessionStorage: storage,
		NoUpdates:      true,
		Device: telegram.DeviceConfig{
			DeviceModel:    "Dispatch",
			SystemVersion:  "self-hosted",
			AppVersion:     "1.2",
			SystemLangCode: "en",
			LangCode:       "en",
		},
	})
	return client.Run(ctx, func(ctx context.Context) error {
		return run(ctx, client)
	})
}

func telegramUserLabel(user *tg.User) string {
	if user.Username != "" {
		return "@" + user.Username
	}
	if name := strings.TrimSpace(user.FirstName + " " + user.LastName); name != "" {
		return name
	}
	return fmt.Sprintf("Telegram user %d", user.ID)
}

func telegramAuthTimeout(api *API) time.Duration {
	if api.Config.ProviderTimeout > 30*time.Second {
		return api.Config.ProviderTimeout
	}
	return 30 * time.Second
}

func (api *API) testTelegramAccount(
	ctx context.Context,
	credentials connectors.Credentials,
) (connectors.TestResult, connectors.Credentials, error) {
	sessionData, err := base64.StdEncoding.DecodeString(credentials.Values["session"])
	if err != nil || len(sessionData) == 0 {
		return connectors.TestResult{}, credentials, errors.New("Telegram session is missing")
	}
	storage := &telegramMemorySession{data: sessionData}
	var status *telegramauth.Status
	err = runTelegramClient(ctx, api.Config.Connectors.TelegramAPIID,
		api.Config.Connectors.TelegramAPIHash, storage, func(ctx context.Context, client *telegram.Client) error {
			var statusErr error
			status, statusErr = client.Auth().Status(ctx)
			return statusErr
		})
	if err != nil {
		return connectors.TestResult{}, credentials, err
	}
	if status == nil || !status.Authorized {
		return connectors.TestResult{}, credentials, errors.New("Telegram session is no longer authorized")
	}
	credentials.Values["session"] = base64.StdEncoding.EncodeToString(storage.Bytes())
	return connectors.TestResult{
		OK: true, AccountLabel: telegramUserLabel(status.User),
		Message: "Telegram account session verified.",
	}, credentials, nil
}
