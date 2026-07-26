package connectors

import (
	"strings"
	"testing"
	"time"
)

func TestParseAndNormalizeYouTubeFeed(t *testing.T) {
	entries, err := ParseYouTubeFeed(strings.NewReader(`
		<feed xmlns="http://www.w3.org/2005/Atom"
		      xmlns:yt="http://www.youtube.com/xml/schemas/2015">
		  <entry>
		    <id>yt:video:video-1</id>
		    <yt:videoId>video-1</yt:videoId>
		    <yt:channelId>UC_channel</yt:channelId>
		    <title>Release notes</title>
		    <link rel="alternate" href="https://www.youtube.com/watch?v=video-1"/>
		    <published>2026-07-26T10:00:00Z</published>
		    <updated>2026-07-26T10:01:00Z</updated>
		    <author><name>Dispatch Channel</name></author>
		  </entry>
		</feed>`))
	if err != nil || len(entries) != 1 {
		t.Fatalf("ParseYouTubeFeed() entries=%#v err=%v", entries, err)
	}
	event, err := NormalizeYouTubeEntry(
		entries[0],
		time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if event.EventType != "youtube.video.published" ||
		!strings.Contains(event.Subject, "Release notes") ||
		!strings.Contains(event.Body, "Dispatch Channel") ||
		event.Metadata["video_id"] != "video-1" {
		t.Fatalf("unexpected normalized event: %#v", event)
	}
}

func TestNormalizeYouTubeEntryDetectsMetadataUpdate(t *testing.T) {
	entry := YouTubeFeedEntry{
		VideoID: "video-1", ChannelID: "UC_channel", Title: "Renamed",
		Published: "2026-07-20T10:00:00Z",
		Updated:   "2026-07-26T10:01:00Z",
	}
	event, err := NormalizeYouTubeEntry(
		entry,
		time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if event.EventType != "youtube.video.updated" {
		t.Fatalf("EventType=%q, want metadata update", event.EventType)
	}
}
