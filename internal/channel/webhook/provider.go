package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"dispatch/internal/config"
	"dispatch/internal/delivery"
)

type Provider struct {
	config config.WebhookConfig
	client *http.Client
	lookup func(context.Context, string) ([]net.IP, error)
}

func New(value config.WebhookConfig, client *http.Client) *Provider {
	if client == nil {
		client = http.DefaultClient
	}
	return &Provider{
		config: value,
		client: client,
		lookup: func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		},
	}
}
func (p *Provider) Channel() delivery.Channel { return delivery.ChannelWebhook }
func (p *Provider) Configured() bool          { return p.config.Enabled }

func (p *Provider) ValidateURL(ctx context.Context, raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return fmt.Errorf("webhook URL must use http or https")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("webhook URL cannot contain credentials or fragments")
	}
	if p.config.AllowPrivate {
		return nil
	}
	addresses, err := p.lookup(ctx, parsed.Hostname())
	if err != nil {
		return fmt.Errorf("resolve webhook host: %w", err)
	}
	for _, address := range addresses {
		if address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() {
			return fmt.Errorf("private webhook destinations are disabled")
		}
	}
	return nil
}

func (p *Provider) Deliver(ctx context.Context, message delivery.Message) (delivery.ProviderResult, error) {
	if err := p.ValidateURL(ctx, message.Destination); err != nil {
		return delivery.ProviderResult{}, &delivery.ProviderError{Code: "webhook_unsafe_url", Message: err.Error()}
	}
	body, _ := json.Marshal(map[string]any{
		"id": message.DeliveryID, "notification_id": message.NotificationID,
		"subject": message.Subject, "body": message.Body, "metadata": message.Metadata,
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, message.Destination, bytes.NewReader(body))
	if err != nil {
		return delivery.ProviderResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", message.DeliveryID)
	request.Header.Set("X-Dispatch-Delivery-ID", message.DeliveryID)
	response, err := p.client.Do(request)
	if err != nil {
		return delivery.ProviderResult{}, &delivery.ProviderError{Code: "webhook_network_error", Message: err.Error(), Retryable: true}
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return delivery.ProviderResult{}, &delivery.ProviderError{
			Code: "webhook_response_error", Message: fmt.Sprintf("status %d: %s", response.StatusCode, strings.TrimSpace(string(raw))),
			Retryable: response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode >= 500,
		}
	}
	return delivery.ProviderResult{ResponseCode: response.StatusCode}, nil
}
