package connectors

import "dispatch/internal/config"

func testConnectorConfig() config.ConnectorConfig {
	return config.ConnectorConfig{PublicURL: "http://localhost:8090"}
}
