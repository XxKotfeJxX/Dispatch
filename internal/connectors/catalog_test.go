package connectors

import (
	"testing"

	"dispatch/internal/config"
)

func testConnectorConfig() config.ConnectorConfig {
	return config.ConnectorConfig{PublicURL: "http://localhost:8090"}
}

func TestCatalogAlwaysReturnsFieldArrays(t *testing.T) {
	for _, manifest := range Catalog(testConnectorConfig()) {
		if manifest.Fields == nil {
			t.Fatalf("%s fields = nil, want an empty array", manifest.ID)
		}
	}
}
