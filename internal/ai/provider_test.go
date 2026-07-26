package ai

import "testing"

func TestValidateDecisionAcceptsAnalysis(t *testing.T) {
	if !ValidateDecision(Decision{
		Category: "ops", Priority: "normal", Summary: "Service recovered", Confidence: .9,
	}) {
		t.Fatal("expected a valid analysis decision")
	}
}

func TestValidateDecisionRejectsInvalidPriority(t *testing.T) {
	if ValidateDecision(Decision{
		Category: "ops", Priority: "urgent", Summary: "Service recovered", Confidence: .9,
	}) {
		t.Fatal("expected invalid priority to be rejected")
	}
}

func TestValidateDecisionRejectsInvalidConfidence(t *testing.T) {
	if ValidateDecision(Decision{
		Category: "ops", Priority: "normal", Summary: "Service recovered", Confidence: 1.1,
	}) {
		t.Fatal("expected invalid confidence to be rejected")
	}
}
