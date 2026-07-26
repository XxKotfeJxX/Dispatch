package connectors

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	GitHubModeImportant = "important"
	GitHubModeCode      = "code"
	GitHubModeWork      = "work"
	GitHubModeCI        = "ci"
	GitHubModeAll       = "all"
)

type GitHubPayload struct {
	Action       string `json:"action"`
	Ref          string `json:"ref"`
	Installation struct {
		ID      int64 `json:"id"`
		Account struct {
			Login string `json:"login"`
		} `json:"account"`
	} `json:"installation"`
	Repository struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
	Issue struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
	} `json:"issue"`
	PullRequest struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
	} `json:"pull_request"`
	Discussion struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
	} `json:"discussion"`
	WorkflowRun struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
		Status     string `json:"status"`
		HTMLURL    string `json:"html_url"`
	} `json:"workflow_run"`
	WorkflowJob struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
		Status     string `json:"status"`
		HTMLURL    string `json:"html_url"`
	} `json:"workflow_job"`
	CheckRun struct {
		Name       string `json:"name"`
		Conclusion string `json:"conclusion"`
		Status     string `json:"status"`
		HTMLURL    string `json:"html_url"`
	} `json:"check_run"`
	Release struct {
		Name    string `json:"name"`
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	} `json:"release"`
	HeadCommit struct {
		Message string `json:"message"`
		URL     string `json:"url"`
	} `json:"head_commit"`
	DeploymentStatus struct {
		State       string `json:"state"`
		Environment string `json:"environment"`
	} `json:"deployment_status"`
}

func VerifyGitHubSignature(secret, signature string, body []byte) bool {
	if secret == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func DecodeGitHubPayload(body []byte) (GitHubPayload, error) {
	var payload GitHubPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return payload, errors.New("invalid GitHub webhook payload")
	}
	if payload.Installation.ID <= 0 {
		return payload, errors.New("GitHub App installation ID is missing")
	}
	return payload, nil
}

func GitHubEventMatches(connection Connection, event string, payload GitHubPayload) bool {
	if connection.ConnectorID != "github" || !connection.Enabled ||
		connection.Config["installation_id"] != fmt.Sprint(payload.Installation.ID) {
		return false
	}
	switch connection.Config["mode"] {
	case GitHubModeAll:
		return true
	case GitHubModeCode:
		return githubEventIn(event,
			"push", "create", "delete", "pull_request", "pull_request_review",
			"pull_request_review_comment", "release")
	case GitHubModeWork:
		return githubEventIn(event,
			"issues", "issue_comment", "discussion", "discussion_comment")
	case GitHubModeCI:
		return githubEventIn(event,
			"workflow_run", "workflow_job", "check_run", "check_suite",
			"deployment", "deployment_status")
	default:
		return githubEventIn(event,
			"issues", "issue_comment", "pull_request", "pull_request_review",
			"workflow_run", "workflow_job", "release")
	}
}

func NormalizeGitHubEvent(event, delivery string, payload GitHubPayload) NormalizedEvent {
	repository := payload.Repository.FullName
	if repository == "" {
		repository = payload.Installation.Account.Login
	}
	action := strings.TrimSpace(payload.Action)
	detail, link := githubEventDetail(event, payload)
	subjectParts := []string{"GitHub", repository, event}
	if action != "" {
		subjectParts = append(subjectParts, action)
	}
	subject := strings.Join(subjectParts, " · ")
	bodyParts := make([]string, 0, 3)
	if detail != "" {
		bodyParts = append(bodyParts, detail)
	}
	if payload.Sender.Login != "" {
		bodyParts = append(bodyParts, "Triggered by @"+payload.Sender.Login)
	}
	if link != "" {
		bodyParts = append(bodyParts, link)
	}
	if len(bodyParts) == 0 {
		bodyParts = append(bodyParts, "GitHub delivered a "+event+" event.")
	}
	return NormalizedEvent{
		ExternalID: delivery,
		EventType:  "github." + event,
		Subject:    subject,
		Body:       strings.Join(bodyParts, "\n"),
		Metadata: map[string]any{
			"connector":       "github",
			"event":           event,
			"action":          action,
			"repository":      payload.Repository.FullName,
			"sender":          payload.Sender.Login,
			"installation_id": payload.Installation.ID,
			"delivery_id":     delivery,
			"url":             link,
		},
	}
}

func githubEventIn(event string, allowed ...string) bool {
	for _, item := range allowed {
		if event == item {
			return true
		}
	}
	return false
}

func githubEventDetail(event string, payload GitHubPayload) (string, string) {
	switch event {
	case "issues", "issue_comment":
		return fmt.Sprintf("#%d %s", payload.Issue.Number, payload.Issue.Title), payload.Issue.HTMLURL
	case "pull_request", "pull_request_review", "pull_request_review_comment":
		return fmt.Sprintf("#%d %s", payload.PullRequest.Number, payload.PullRequest.Title), payload.PullRequest.HTMLURL
	case "discussion", "discussion_comment":
		return fmt.Sprintf("#%d %s", payload.Discussion.Number, payload.Discussion.Title), payload.Discussion.HTMLURL
	case "workflow_run":
		return githubStatusDetail(payload.WorkflowRun.Name, payload.WorkflowRun.Status,
			payload.WorkflowRun.Conclusion), payload.WorkflowRun.HTMLURL
	case "workflow_job":
		return githubStatusDetail(payload.WorkflowJob.Name, payload.WorkflowJob.Status,
			payload.WorkflowJob.Conclusion), payload.WorkflowJob.HTMLURL
	case "check_run":
		return githubStatusDetail(payload.CheckRun.Name, payload.CheckRun.Status,
			payload.CheckRun.Conclusion), payload.CheckRun.HTMLURL
	case "release":
		name := payload.Release.Name
		if name == "" {
			name = payload.Release.TagName
		}
		return name, payload.Release.HTMLURL
	case "push":
		return payload.HeadCommit.Message, payload.HeadCommit.URL
	case "deployment_status":
		return githubStatusDetail(payload.DeploymentStatus.Environment,
			payload.DeploymentStatus.State, ""), payload.Repository.HTMLURL
	default:
		return "", payload.Repository.HTMLURL
	}
}

func githubStatusDetail(name, status, conclusion string) string {
	result := strings.TrimSpace(name)
	if status != "" {
		result += " — " + status
	}
	if conclusion != "" {
		result += " (" + conclusion + ")"
	}
	return strings.TrimSpace(result)
}
