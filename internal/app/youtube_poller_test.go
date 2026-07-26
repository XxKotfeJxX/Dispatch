package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dispatch/internal/config"
)

func TestYouTubeSubscriptionPaginationAndFeedFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/subscriptions":
			if request.Header.Get("Authorization") != "Bearer access-token" ||
				request.URL.Query().Get("mine") != "true" {
				t.Fatalf("unexpected subscription request: %s", request.URL.String())
			}
			writer.Header().Set("Content-Type", "application/json")
			if request.URL.Query().Get("pageToken") == "" {
				_, _ = fmt.Fprint(writer, `{
					"items":[{"snippet":{"title":"One","resourceId":{"channelId":"UC_one"}}}],
					"nextPageToken":"next"
				}`)
				return
			}
			_, _ = fmt.Fprint(writer, `{
				"items":[{"snippet":{"title":"Two","resourceId":{"channelId":"UC_two"}}}]
			}`)
		case "/feeds/videos.xml":
			if request.URL.Query().Get("channel_id") != "UC_one" {
				t.Fatalf("unexpected feed request: %s", request.URL.String())
			}
			writer.Header().Set("Content-Type", "application/atom+xml")
			_, _ = fmt.Fprint(writer, `
				<feed xmlns="http://www.w3.org/2005/Atom"
				      xmlns:yt="http://www.youtube.com/xml/schemas/2015">
				  <entry>
				    <yt:videoId>video-1</yt:videoId>
				    <yt:channelId>UC_one</yt:channelId>
				    <title>New video</title>
				    <published>2026-07-26T10:00:00Z</published>
				    <updated>2026-07-26T10:00:00Z</updated>
				  </entry>
				</feed>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	worker := Worker{Config: config.Config{
		ProviderTimeout: time.Second,
		Connectors: config.ConnectorConfig{
			YouTubeAPIBase:  server.URL,
			YouTubeFeedBase: server.URL,
		},
	}}
	subscriptions, err := worker.listYouTubeSubscriptions(t.Context(), "access-token")
	if err != nil || len(subscriptions) != 2 ||
		subscriptions[0].ChannelID != "UC_one" ||
		subscriptions[1].ChannelID != "UC_two" {
		t.Fatalf("subscriptions=%#v err=%v", subscriptions, err)
	}
	entries, err := worker.fetchYouTubeFeed(t.Context(), "UC_one")
	if err != nil || len(entries) != 1 || entries[0].VideoID != "video-1" {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
}

func TestYouTubeChannelCursorsRejectsMalformedState(t *testing.T) {
	if cursors := youtubeChannelCursors(`{"UC_one":"2026-07-26T10:00:00Z"}`); cursors["UC_one"] == "" {
		t.Fatalf("valid cursor state was lost: %#v", cursors)
	}
	if cursors := youtubeChannelCursors(`not-json`); len(cursors) != 0 {
		t.Fatalf("malformed cursor state must reset safely: %#v", cursors)
	}
}
