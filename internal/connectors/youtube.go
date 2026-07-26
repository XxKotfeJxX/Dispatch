package connectors

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type YouTubeFeedEntry struct {
	ID        string `xml:"id"`
	VideoID   string `xml:"videoId"`
	ChannelID string `xml:"channelId"`
	Title     string `xml:"title"`
	Published string `xml:"published"`
	Updated   string `xml:"updated"`
	Link      struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Author struct {
		Name string `xml:"name"`
	} `xml:"author"`
}

func ParseYouTubeFeed(reader io.Reader) ([]YouTubeFeedEntry, error) {
	var feed struct {
		Entries []YouTubeFeedEntry `xml:"entry"`
	}
	if err := xml.NewDecoder(reader).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decode YouTube feed: %w", err)
	}
	return feed.Entries, nil
}

func YouTubeEntryTime(entry YouTubeFeedEntry) (time.Time, error) {
	value := strings.TrimSpace(entry.Updated)
	if value == "" {
		value = strings.TrimSpace(entry.Published)
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid YouTube entry timestamp %q", value)
	}
	return parsed.UTC(), nil
}

func NormalizeYouTubeEntry(
	entry YouTubeFeedEntry,
	previousCursor time.Time,
) (NormalizedEvent, error) {
	if strings.TrimSpace(entry.VideoID) == "" ||
		strings.TrimSpace(entry.ChannelID) == "" {
		return NormalizedEvent{}, errors.New("YouTube feed entry omitted video or channel ID")
	}
	updatedAt, err := YouTubeEntryTime(entry)
	if err != nil {
		return NormalizedEvent{}, err
	}
	publishedAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(entry.Published))
	eventType := "youtube.video.updated"
	action := "Updated YouTube video"
	if !publishedAt.IsZero() && publishedAt.After(previousCursor) {
		eventType = "youtube.video.published"
		action = "New YouTube video"
	}
	channelName := strings.TrimSpace(entry.Author.Name)
	if channelName == "" {
		channelName = entry.ChannelID
	}
	videoURL := strings.TrimSpace(entry.Link.Href)
	if videoURL == "" {
		videoURL = "https://www.youtube.com/watch?v=" + entry.VideoID
	}
	title := strings.TrimSpace(entry.Title)
	if title == "" {
		title = entry.VideoID
	}
	publishedText := strings.TrimSpace(entry.Published)
	if publishedText == "" {
		publishedText = updatedAt.Format(time.RFC3339)
	}
	return NormalizedEvent{
		ExternalID: entry.VideoID + ":" + updatedAt.Format(time.RFC3339Nano),
		EventType:  eventType,
		Subject:    action + ": " + title,
		Body: strings.Join([]string{
			"Channel: " + channelName,
			"Published: " + publishedText,
			"Watch: " + videoURL,
		}, "\n"),
		Metadata: map[string]any{
			"connector":    "youtube",
			"video_id":     entry.VideoID,
			"channel_id":   entry.ChannelID,
			"channel_name": channelName,
			"video_url":    videoURL,
			"published_at": entry.Published,
			"updated_at":   entry.Updated,
		},
	}, nil
}
