package connectors

import (
	"net/http"
	"testing"
)

func TestValidateYouTubeChannel(t *testing.T) {
	manifest, ok := Find(testConnectorConfig(), "youtube")
	if !ok {
		t.Fatal("YouTube manifest missing")
	}
	if err := ValidateInput(manifest, map[string]string{"channel_id": "invalid"}, nil); err == nil {
		t.Fatal("invalid channel accepted")
	}
	if err := ValidateInput(manifest,
		map[string]string{"channel_id": "UC_x5XG1OV2P6uZZ5FSM9Ttw"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDemoActivationDoesNotRequirePublicHTTPS(t *testing.T) {
	status, err := (Service{PublicURL: "http://localhost:8090"}).Activate(
		t.Context(), Connection{ConnectorID: "demo"}, Credentials{},
	)
	if err != nil || status != "connected" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestNormalizeTelegram(t *testing.T) {
	connection := Connection{ConnectorID: "telegram"}
	credentials := Credentials{Values: map[string]string{"webhook_secret": "secret"}}
	headers := http.Header{"X-Telegram-Bot-Api-Secret-Token": {"secret"}}
	event, err := VerifyAndNormalize(connection, credentials, headers, []byte(`{
		"update_id":42,
		"message":{"message_id":7,"text":"hello","chat":{"id":8},
		"from":{"id":9,"first_name":"Ada","username":"ada"}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.ExternalID != "42" || event.EventType != "telegram.message" ||
		event.Subject != "@ada" || event.Body != "hello" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestNormalizeYouTubeRequiresConfiguredChannel(t *testing.T) {
	connection := Connection{
		ConnectorID: "youtube", Config: map[string]string{"channel_id": "UC_expected"},
	}
	_, err := VerifyAndNormalize(connection, Credentials{}, nil, []byte(`
		<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015">
		  <entry><id>video</id><yt:videoId>abc</yt:videoId>
		  <yt:channelId>UC_other</yt:channelId><title>Video</title></entry>
		</feed>`))
	if err == nil {
		t.Fatal("mismatched YouTube channel accepted")
	}
}
