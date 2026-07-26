package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"dispatch/internal/channel/email"
	"dispatch/internal/connectors"
	"dispatch/internal/delivery"
	"dispatch/internal/recipient"
	"dispatch/internal/store/postgres"
)

var mailpitLoginPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}$`)

func (api *API) startMailpitRecipientSetup(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Name  string `json:"name"`
		Login string `json:"login"`
	}
	if !decode(writer, request, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Login = strings.ToLower(strings.TrimSpace(input.Login))
	if input.Name == "" || !mailpitLoginPattern.MatchString(input.Login) {
		writeError(writer, http.StatusBadRequest, "validation_error",
			"name and a valid mailbox name are required")
		return
	}
	provider := email.New(api.Config.MailpitSMTP)
	if !provider.Configured() {
		writeError(writer, http.StatusConflict, "mailpit_not_configured",
			"the local Mailpit SMTP service is not configured")
		return
	}
	code, err := numericCode()
	if err != nil {
		api.storeError(writer, err)
		return
	}
	item := recipient.Setup{
		ID:   "rst_" + uuid.NewString(),
		Kind: "mailpit", Name: input.Name,
		Target:    input.Login + "@dispatch.local",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}
	item.CodeHash = setupCodeHash(item.ID, code)
	if err := api.Store.CreateRecipientSetup(request.Context(), &item); err != nil {
		api.storeError(writer, err)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), api.Config.ProviderTimeout)
	defer cancel()
	_, err = provider.Deliver(ctx, delivery.Message{
		DeliveryID: "setup", NotificationID: item.ID, Destination: item.Target,
		Subject: "Your Dispatch Mailpit verification code",
		Body: fmt.Sprintf(
			"Enter this code in Dispatch:\n\n%s\n\nIt expires in 10 minutes.", code,
		),
	})
	if err != nil {
		writeError(writer, http.StatusBadGateway, "verification_delivery_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{
		"data": item, "mailpit_url": api.Config.MailpitPublicURL,
	})
}

func (api *API) verifyMailpitRecipientSetup(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	if !decode(writer, request, &input) {
		return
	}
	id := chi.URLParam(request, "id")
	code := strings.TrimSpace(input.Code)
	if len(code) != 6 {
		writeError(writer, http.StatusBadRequest, "validation_error",
			"enter the six-digit code from Mailpit")
		return
	}
	item, err := api.Store.CompleteMailpitRecipientSetup(
		request.Context(), id, setupCodeHash(id, code),
	)
	if err != nil {
		if errors.Is(err, postgres.ErrInvalidState) || errors.Is(err, postgres.ErrNotFound) {
			writeError(writer, http.StatusConflict, "verification_failed",
				"the code is invalid, expired, or has already been used")
			return
		}
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"data": item})
}

func (api *API) startTelegramRecipientSetup(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if !decode(writer, request, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		writeError(writer, http.StatusBadRequest, "validation_error", "name is required")
		return
	}
	if api.Config.Telegram.Token == "" {
		writeError(writer, http.StatusConflict, "telegram_not_configured",
			"the Dispatch Telegram delivery bot is not configured")
		return
	}
	code, err := connectors.RandomURLToken(18)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	item := recipient.Setup{
		ID: "rst_" + uuid.NewString(), Kind: "telegram", Name: input.Name,
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	}
	item.CodeHash = setupCodeHash("", code)
	username, err := api.telegramBotUsername(request.Context())
	if err != nil {
		writeError(writer, http.StatusBadGateway, "telegram_bot_unavailable", err.Error())
		return
	}
	if err := api.Store.CreateRecipientSetup(request.Context(), &item); err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{
		"data":    item,
		"bot_url": "https://t.me/" + url.PathEscape(username) + "?start=" + url.QueryEscape(code),
	})
}

func (api *API) getRecipientSetup(writer http.ResponseWriter, request *http.Request) {
	item, err := api.Store.GetRecipientSetup(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": item})
}

func (api *API) telegramBotUsername(ctx context.Context) (string, error) {
	endpoint := strings.TrimRight(api.Config.Telegram.APIBase, "/") +
		"/bot" + api.Config.Telegram.Token + "/getMe"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	response, err := (&http.Client{Timeout: api.Config.ProviderTimeout}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Telegram getMe returned %s: %s",
			response.Status, strings.TrimSpace(string(raw)))
	}
	var payload struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if !payload.OK || payload.Result.Username == "" {
		return "", fmt.Errorf("Telegram bot username is unavailable")
	}
	return payload.Result.Username, nil
}

func numericCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func setupCodeHash(id, code string) []byte {
	sum := sha256.Sum256([]byte(id + ":" + code))
	return sum[:]
}
