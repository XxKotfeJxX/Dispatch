package connectors

import (
	"testing"
)

func TestDemoActivationDoesNotRequirePublicHTTPS(t *testing.T) {
	status, err := (Service{PublicURL: "http://localhost:8090"}).Activate(
		t.Context(), Connection{ConnectorID: "demo"}, Credentials{},
	)
	if err != nil || status != "connected" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}
