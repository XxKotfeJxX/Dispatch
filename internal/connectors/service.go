package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var youtubeChannelPattern = regexp.MustCompile(`^UC[A-Za-z0-9_-]{20,30}$`)

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
	case "youtube":
		return TestResult{
			OK: true, AccountLabel: config["channel_id"],
			Message: "YouTube channel is valid and ready for WebSub activation.",
		}, nil
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
	if manifest.ID == "youtube" && !youtubeChannelPattern.MatchString(config["channel_id"]) {
		return errors.New("YouTube channel ID must start with UC and contain a valid channel identifier")
	}
	return nil
}

func (service Service) Activate(
	ctx context.Context,
	connection Connection,
	credentials Credentials,
) (string, error) {
	if connection.ConnectorID == "youtube" &&
		!strings.HasPrefix(strings.ToLower(service.PublicURL), "https://") {
		return "action_required", errors.New("live callbacks require CONNECTOR_PUBLIC_URL with public HTTPS")
	}
	callback := service.PublicURL + "/connect/v1/hooks/" + url.PathEscape(connection.ID)
	if connection.ConnectorID == "youtube" {
		callback += "?token=" + url.QueryEscape(credentials.Values["hook_secret"])
	}
	switch connection.ConnectorID {
	case "youtube":
		values := url.Values{
			"hub.callback": {callback},
			"hub.mode":     {"subscribe"},
			"hub.topic": {
				"https://www.youtube.com/feeds/videos.xml?channel_id=" + connection.Config["channel_id"],
			},
			"hub.verify": {"async"},
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://pubsubhubbub.appspot.com/subscribe", strings.NewReader(values.Encode()))
		if err != nil {
			return "error", err
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := service.client().Do(request)
		if err != nil {
			return "error", err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return "error", fmt.Errorf("YouTube hub returned %s", response.Status)
		}
		return "action_required", nil
	default:
		return "connected", nil
	}
}

func (service Service) Deactivate(
	ctx context.Context,
	connection Connection,
	credentials Credentials,
) error {
	callback := service.PublicURL + "/connect/v1/hooks/" + url.PathEscape(connection.ID)
	if connection.ConnectorID == "youtube" {
		callback += "?token=" + url.QueryEscape(credentials.Values["hook_secret"])
	}
	switch connection.ConnectorID {
	case "youtube":
		if !strings.HasPrefix(strings.ToLower(service.PublicURL), "https://") {
			return nil
		}
		values := url.Values{
			"hub.callback": {callback},
			"hub.mode":     {"unsubscribe"},
			"hub.topic": {
				"https://www.youtube.com/feeds/videos.xml?channel_id=" + connection.Config["channel_id"],
			},
			"hub.verify": {"async"},
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://pubsubhubbub.appspot.com/subscribe", strings.NewReader(values.Encode()))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err := service.client().Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("YouTube hub returned %s", response.Status)
		}
	}
	return nil
}

func VerifyAndNormalize(
	connection Connection,
	credentials Credentials,
	headers http.Header,
	body []byte,
) (NormalizedEvent, error) {
	switch connection.ConnectorID {
	case "youtube":
		var feed struct {
			Entries []struct {
				ID        string `xml:"id"`
				Title     string `xml:"title"`
				VideoID   string `xml:"videoId"`
				ChannelID string `xml:"channelId"`
				Link      struct {
					Href string `xml:"href,attr"`
				} `xml:"link"`
			} `xml:"entry"`
		}
		if err := xml.Unmarshal(body, &feed); err != nil || len(feed.Entries) == 0 {
			return NormalizedEvent{}, errors.New("invalid YouTube Atom notification")
		}
		entry := feed.Entries[0]
		if configured := connection.Config["channel_id"]; configured != "" &&
			entry.ChannelID != configured {
			return NormalizedEvent{}, errors.New("YouTube channel does not match the connection")
		}
		externalID := entry.VideoID
		if externalID == "" {
			externalID = entry.ID
		}
		return NormalizedEvent{
			ExternalID: externalID, EventType: "youtube.video.updated",
			Subject: entry.Title, Body: entry.Link.Href,
			Metadata: map[string]any{
				"connector": "youtube", "video_id": entry.VideoID,
				"channel_id": entry.ChannelID,
			},
		}, nil
	default:
		return NormalizedEvent{}, errors.New("connector does not accept webhook events")
	}
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
