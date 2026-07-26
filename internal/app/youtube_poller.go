package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dispatch/internal/connectors"
)

type youtubeSubscription struct {
	ChannelID string
	Title     string
}

type youtubeSubscriptionsResponse struct {
	Items []struct {
		Snippet struct {
			Title      string `json:"title"`
			ResourceID struct {
				ChannelID string `json:"channelId"`
			} `json:"resourceId"`
		} `json:"snippet"`
	} `json:"items"`
	NextPageToken string `json:"nextPageToken"`
}

type youtubeFeedResult struct {
	Subscription youtubeSubscription
	Entries      []connectors.YouTubeFeedEntry
	Err          error
}

func (worker *Worker) youtubeLoop(ctx context.Context) {
	worker.pollYouTubeConnections(ctx)
	ticker := time.NewTicker(worker.Config.Connectors.YouTubePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			worker.pollYouTubeConnections(ctx)
		}
	}
}

func (worker *Worker) pollYouTubeConnections(ctx context.Context) {
	items, err := worker.Store.ListConnectorConnections(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			worker.Logger.Error("list YouTube connections", "error", err)
		}
		return
	}
	for _, item := range items {
		if item.ConnectorID != "youtube" || !item.Enabled {
			continue
		}
		if err := worker.pollYouTubeConnection(ctx, item.ID); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			worker.Logger.Warn(
				"poll YouTube subscriptions",
				"connection_id", item.ID,
				"error", err,
			)
			_ = worker.Store.UpdateConnectorState(
				ctx, item.ID, "error", "", truncateConnectorError(err.Error()), true,
			)
		}
	}
}

func (worker *Worker) pollYouTubeConnection(ctx context.Context, connectionID string) error {
	startedAt := time.Now().UTC()
	connection, credentials, err := worker.loadGoogleConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	subscriptions, err := worker.listYouTubeSubscriptions(ctx, credentials.AccessToken)
	if err != nil {
		return err
	}
	cursors := youtubeChannelCursors(connection.Config["youtube_channel_cursors"])
	active := make(map[string]youtubeSubscription, len(subscriptions))
	changed := false
	for _, subscription := range subscriptions {
		active[subscription.ChannelID] = subscription
		if _, exists := cursors[subscription.ChannelID]; !exists {
			// Establish a current cursor so connecting YouTube never floods Dispatch
			// with old uploads from every existing subscription.
			cursors[subscription.ChannelID] = startedAt.Format(time.RFC3339Nano)
			changed = true
		}
	}
	for channelID := range cursors {
		if _, subscribed := active[channelID]; !subscribed {
			delete(cursors, channelID)
			changed = true
		}
	}

	results := worker.fetchYouTubeFeeds(ctx, subscriptions, cursors, startedAt)
	warnings := make([]string, 0)
	for _, result := range results {
		if result.Err != nil {
			warnings = append(warnings, result.Subscription.Title+": "+result.Err.Error())
			continue
		}
		cursor, err := time.Parse(time.RFC3339Nano, cursors[result.Subscription.ChannelID])
		if err != nil {
			cursor = startedAt
		}
		sort.Slice(result.Entries, func(left, right int) bool {
			leftTime, _ := connectors.YouTubeEntryTime(result.Entries[left])
			rightTime, _ := connectors.YouTubeEntryTime(result.Entries[right])
			return leftTime.Before(rightTime)
		})
		processed := true
		for _, entry := range result.Entries {
			entryTime, entryErr := connectors.YouTubeEntryTime(entry)
			if entryErr != nil {
				warnings = append(warnings, result.Subscription.Title+": "+entryErr.Error())
				continue
			}
			if !entryTime.After(cursor) {
				continue
			}
			event, normalizeErr := connectors.NormalizeYouTubeEntry(entry, cursor)
			if normalizeErr != nil {
				warnings = append(warnings, result.Subscription.Title+": "+normalizeErr.Error())
				continue
			}
			if enqueueErr := worker.enqueueGoogleEvent(ctx, connection, event); enqueueErr != nil {
				warnings = append(warnings, result.Subscription.Title+": "+enqueueErr.Error())
				processed = false
				break
			}
		}
		if processed {
			cursors[result.Subscription.ChannelID] = startedAt.Format(time.RFC3339Nano)
			changed = true
		}
	}

	connection.Config["youtube_subscription_count"] = strconv.Itoa(len(subscriptions))
	if changed || connection.Config["youtube_channel_cursors"] == "" {
		raw, err := json.Marshal(cursors)
		if err != nil {
			return err
		}
		connection.Config["youtube_channel_cursors"] = string(raw)
	}
	if err := worker.Store.UpdateConnectorConfig(
		ctx, connection.ID, connection.Config,
	); err != nil {
		return err
	}
	lastError := ""
	if len(warnings) > 0 {
		lastError = truncateConnectorError(strings.Join(warnings, "; "))
	}
	return worker.Store.UpdateConnectorState(
		ctx, connection.ID, "connected", "", lastError, true,
	)
}

func (worker *Worker) listYouTubeSubscriptions(
	ctx context.Context,
	accessToken string,
) ([]youtubeSubscription, error) {
	result := make([]youtubeSubscription, 0)
	pageToken := ""
	for {
		values := url.Values{
			"part":       {"snippet"},
			"mine":       {"true"},
			"maxResults": {"50"},
		}
		if pageToken != "" {
			values.Set("pageToken", pageToken)
		}
		var response youtubeSubscriptionsResponse
		if err := worker.googleJSON(
			ctx, http.MethodGet, worker.Config.Connectors.YouTubeAPIBase,
			"/subscriptions?"+values.Encode(), accessToken, nil, &response,
		); err != nil {
			return nil, err
		}
		for _, item := range response.Items {
			channelID := strings.TrimSpace(item.Snippet.ResourceID.ChannelID)
			if channelID == "" {
				continue
			}
			result = append(result, youtubeSubscription{
				ChannelID: channelID,
				Title:     strings.TrimSpace(item.Snippet.Title),
			})
		}
		pageToken = response.NextPageToken
		if pageToken == "" {
			return result, nil
		}
	}
}

func (worker *Worker) fetchYouTubeFeeds(
	ctx context.Context,
	subscriptions []youtubeSubscription,
	cursors map[string]string,
	startedAt time.Time,
) []youtubeFeedResult {
	results := make(chan youtubeFeedResult, len(subscriptions))
	semaphore := make(chan struct{}, 8)
	var group sync.WaitGroup
	for _, subscription := range subscriptions {
		cursor, err := time.Parse(time.RFC3339Nano, cursors[subscription.ChannelID])
		if err != nil || !cursor.Before(startedAt) {
			continue
		}
		group.Add(1)
		go func(item youtubeSubscription) {
			defer group.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				results <- youtubeFeedResult{Subscription: item, Err: ctx.Err()}
				return
			}
			defer func() { <-semaphore }()
			entries, fetchErr := worker.fetchYouTubeFeed(ctx, item.ChannelID)
			results <- youtubeFeedResult{
				Subscription: item,
				Entries:      entries,
				Err:          fetchErr,
			}
		}(subscription)
	}
	group.Wait()
	close(results)
	collected := make([]youtubeFeedResult, 0, len(subscriptions))
	for result := range results {
		collected = append(collected, result)
	}
	return collected
}

func (worker *Worker) fetchYouTubeFeed(
	ctx context.Context,
	channelID string,
) ([]connectors.YouTubeFeedEntry, error) {
	requestCtx, cancel := context.WithTimeout(ctx, worker.Config.ProviderTimeout)
	defer cancel()
	endpoint := strings.TrimRight(worker.Config.Connectors.YouTubeFeedBase, "/") +
		"/feeds/videos.xml?channel_id=" + url.QueryEscape(channelID)
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := (&http.Client{Timeout: worker.Config.ProviderTimeout}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return nil, fmt.Errorf(
			"YouTube feed returned %s: %s",
			response.Status,
			strings.TrimSpace(string(raw)),
		)
	}
	return connectors.ParseYouTubeFeed(io.LimitReader(response.Body, 2<<20))
}

func youtubeChannelCursors(value string) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(value) != "" {
		_ = json.Unmarshal([]byte(value), &result)
	}
	return result
}

func truncateConnectorError(value string) string {
	if len(value) > 2000 {
		return value[:2000]
	}
	return value
}
