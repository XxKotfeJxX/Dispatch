package connectors

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"installation":{"id":42}}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !VerifyGitHubSignature("secret", signature, body) {
		t.Fatal("valid GitHub signature was rejected")
	}
	if VerifyGitHubSignature("other", signature, body) {
		t.Fatal("invalid GitHub signature was accepted")
	}
}

func TestGitHubEventMatchesInstallationAndMode(t *testing.T) {
	connection := Connection{
		ConnectorID: "github",
		Enabled:     true,
		Config: map[string]string{
			"installation_id": "42",
			"mode":            GitHubModeCI,
		},
	}
	var payload GitHubPayload
	payload.Installation.ID = 42
	if !GitHubEventMatches(connection, "workflow_run", payload) {
		t.Fatal("CI event should match")
	}
	if GitHubEventMatches(connection, "issues", payload) {
		t.Fatal("issue event should not match CI mode")
	}
	payload.Installation.ID = 43
	if GitHubEventMatches(connection, "workflow_run", payload) {
		t.Fatal("another installation must not match")
	}
}

func TestNormalizeGitHubPullRequest(t *testing.T) {
	var payload GitHubPayload
	payload.Installation.ID = 42
	payload.Repository.FullName = "owner/repo"
	payload.PullRequest.Number = 12
	payload.PullRequest.Title = "Ship it"
	payload.PullRequest.HTMLURL = "https://github.com/owner/repo/pull/12"
	payload.Action = "opened"
	event := NormalizeGitHubEvent("pull_request", "delivery-1", payload)
	if event.ExternalID != "delivery-1" || event.EventType != "github.pull_request" ||
		event.Subject == "" || event.Body == "" {
		t.Fatalf("unexpected normalized event: %#v", event)
	}
}
