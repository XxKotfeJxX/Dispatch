package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Service struct {
	Client    *http.Client
	PublicURL string
}

func (service Service) Test(
	ctx context.Context,
	manifest Manifest,
	config, credentials map[string]string,
) (TestResult, error) {
	if err := ValidateInput(manifest, config, credentials); err != nil {
		return TestResult{}, err
	}
	switch manifest.ID {
	case "demo":
		return TestResult{OK: true, AccountLabel: "Local demo", Message: "Demo connector is ready."}, nil
	case "discord":
		var response struct {
			Username string `json:"username"`
			ID       string `json:"id"`
		}
		headers := http.Header{"Authorization": {"Bot " + credentials["bot_token"]}}
		if err := service.jsonRequest(ctx, http.MethodGet,
			"https://discord.com/api/v10/users/@me", headers, nil, &response); err != nil {
			return TestResult{}, fmt.Errorf("Discord verification failed: %w", err)
		}
		return TestResult{OK: true, AccountLabel: response.Username, Message: "Discord bot credentials verified."}, nil
	default:
		return TestResult{}, fmt.Errorf("connector %q cannot be tested with direct credentials", manifest.ID)
	}
}

func ValidateInput(manifest Manifest, config, credentials map[string]string) error {
	if manifest.Availability == Unavailable {
		return errors.New("this connector is not available through an official provider API")
	}
	for _, field := range manifest.Fields {
		value := config[field.Name]
		if field.Secret {
			value = credentials[field.Name]
		}
		if field.Required && strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field.Label)
		}
	}
	return nil
}

func (service Service) Activate(
	ctx context.Context,
	connection Connection,
	credentials Credentials,
) (string, error) {
	return "connected", nil
}

func (service Service) Deactivate(
	ctx context.Context,
	connection Connection,
	credentials Credentials,
) error {
	return nil
}

func (service Service) jsonRequest(
	ctx context.Context,
	method, endpoint string,
	headers http.Header,
	body []byte,
	target any,
) error {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := service.client().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider returned %s", response.Status)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode provider response: %w", err)
	}
	return nil
}

func (service Service) client() *http.Client {
	if service.Client != nil {
		return service.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}
