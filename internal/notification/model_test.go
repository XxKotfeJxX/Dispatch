package notification

import "testing"

func TestTerminalTransitionsAreRejected(t *testing.T) {
	if CanTransition(StatusDelivered, StatusQueued) {
		t.Fatal("delivered notification must remain terminal")
	}
	if !CanTransition(StatusFailed, StatusQueued) {
		t.Fatal("failed notification must support manual retry")
	}
}
