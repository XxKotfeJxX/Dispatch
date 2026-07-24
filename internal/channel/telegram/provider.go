package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"dispatch/internal/config"
	"dispatch/internal/delivery"
)

type Provider struct {
	config config.TelegramConfig
	client *http.Client
}

func New(value config.TelegramConfig, client *http.Client) *Provider {
	if client == nil {
		client = http.DefaultClient
	}
	return &Provider{config: value, client: client}
}
func (p *Provider) Channel() delivery.Channel { return delivery.ChannelTelegram }
func (p *Provider) Configured() bool          { return p.config.Token != "" }

func (p *Provider) Deliver(ctx context.Context, message delivery.Message) (delivery.ProviderResult, error) {
	body, _ := json.Marshal(map[string]any{"chat_id": message.Destination, "text": strings.TrimSpace(message.Subject + "\n\n" + message.Body)})
	url := strings.TrimRight(p.config.APIBase, "/") + "/bot" + p.config.Token + "/sendMessage"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return delivery.ProviderResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return delivery.ProviderResult{}, &delivery.ProviderError{Code: "telegram_network_error", Message: err.Error(), Retryable: true}
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return delivery.ProviderResult{}, &delivery.ProviderError{
			Code: "telegram_provider_error", Message: fmt.Sprintf("status %d: %s", response.StatusCode, strings.TrimSpace(string(raw))),
			Retryable: response.StatusCode == 429 || response.StatusCode >= 500,
		}
	}
	var decoded struct {
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	_ = json.Unmarshal(raw, &decoded)
	return delivery.ProviderResult{ResponseCode: response.StatusCode, ProviderID: fmt.Sprint(decoded.Result.MessageID)}, nil
}
