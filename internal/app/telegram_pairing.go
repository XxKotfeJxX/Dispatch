package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dispatch/internal/delivery"
	"dispatch/internal/store/postgres"
)

type telegramBotUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"chat"`
	} `json:"message"`
}

func (worker *Worker) telegramDeliveryPairingLoop(ctx context.Context) {
	var offset int64
	for ctx.Err() == nil {
		updates, err := worker.telegramBotUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() == nil {
				worker.Logger.Warn("poll Telegram delivery pairing", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if update.Message == nil {
				continue
			}
			code := telegramStartCode(update.Message.Text)
			if code == "" {
				continue
			}
			label := strings.TrimSpace(update.Message.Chat.FirstName + " " + update.Message.Chat.LastName)
			if update.Message.Chat.Username != "" {
				label = "@" + update.Message.Chat.Username
			}
			item, err := worker.Store.CompleteTelegramRecipientSetup(
				ctx, setupCodeHash("", code),
				strconv.FormatInt(update.Message.Chat.ID, 10), label,
			)
			if err != nil {
				if !errors.Is(err, postgres.ErrNotFound) &&
					!errors.Is(err, postgres.ErrInvalidState) {
					worker.Logger.Warn("complete Telegram delivery pairing", "error", err)
				}
				continue
			}
			if provider := worker.Providers[delivery.ChannelTelegram]; provider != nil {
				providerCtx, cancel := context.WithTimeout(ctx, worker.Config.ProviderTimeout)
				_, _ = provider.Deliver(providerCtx, delivery.Message{
					DeliveryID: "pairing", NotificationID: item.ID,
					Destination: item.TelegramChatID,
					Subject:     "Dispatch connected",
					Body:        "This chat is now ready to receive your routed notifications.",
				})
				cancel()
			}
		}
	}
}

func (worker *Worker) telegramBotUpdates(
	ctx context.Context,
	offset int64,
) ([]telegramBotUpdate, error) {
	values := url.Values{
		"timeout":         {"20"},
		"offset":          {strconv.FormatInt(offset, 10)},
		"allowed_updates": {`["message"]`},
	}
	endpoint := strings.TrimRight(worker.Config.Telegram.APIBase, "/") +
		"/bot" + worker.Config.Telegram.Token + "/getUpdates?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := (&http.Client{Timeout: 25 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Telegram getUpdates returned %s", response.Status)
	}
	var payload struct {
		OK     bool                `json:"ok"`
		Result []telegramBotUpdate `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("Telegram getUpdates returned ok=false")
	}
	return payload.Result, nil
}

func telegramStartCode(text string) string {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "/start") {
		return ""
	}
	return parts[1]
}
