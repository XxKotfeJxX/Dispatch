package ai

import (
	"context"
	"strings"
)

type MockDecisionProvider struct {
	Decision Decision
	Err      error
}

func (provider MockDecisionProvider) Name() string  { return "mock" }
func (provider MockDecisionProvider) Model() string { return "mock-analysis-v2" }

func (provider MockDecisionProvider) Decide(_ context.Context, input DecisionInput) (Decision, map[string]any, error) {
	if provider.Err != nil {
		return Decision{}, nil, provider.Err
	}
	if provider.Decision.Category != "" {
		return provider.Decision, map[string]any{"mock": true}, nil
	}
	text := strings.ToLower(input.EventType + " " + input.Subject + " " + input.Body)
	decision := Decision{
		Category: "general", Priority: "normal", Summary: input.Subject,
		Confidence: 0.86, ReasonCodes: []string{"mock_classification"},
	}
	switch {
	case strings.Contains(text, "payment") && strings.Contains(text, "fail"):
		decision.Category, decision.Priority = "payment_failure", "critical"
		decision.ReasonCodes = []string{"financial_event", "repeated_failure"}
	case strings.Contains(text, "incident") || strings.Contains(text, "server"):
		decision.Category, decision.Priority = "infrastructure_incident", "high"
		decision.ReasonCodes = []string{"service_impact"}
	case strings.Contains(text, "report"):
		decision.Category, decision.Priority = "scheduled_report", "low"
		decision.ReasonCodes = []string{"informational"}
	}
	return decision, map[string]any{"mock": true, "classification": decision.Category}, nil
}
