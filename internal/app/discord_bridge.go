package app

import (
	"crypto/subtle"
	"net/http"

	"dispatch/internal/connectors"
)

func (api *API) discordBridgeEvent(writer http.ResponseWriter, request *http.Request) {
	expected := connectors.DiscordBridgeKey(api.Config.Connectors.EncryptionKey)
	provided := request.Header.Get("X-Dispatch-Bridge-Key")
	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		writeError(writer, http.StatusUnauthorized, "invalid_bridge_key", "Discord bridge authentication failed")
		return
	}
	var event connectors.DiscordEvent
	if !decode(writer, request, &event) {
		return
	}
	if event.ID == "" || event.Author.ID == "" || event.ChannelID == "" {
		writeError(writer, http.StatusBadRequest, "validation_error",
			"id, author.id, and channel_id are required")
		return
	}
	connections, err := api.Store.ListConnectorConnections(request.Context())
	if err != nil {
		api.storeError(writer, err)
		return
	}
	normalized := connectors.NormalizeDiscordEvent(event)
	accepted := 0
	notificationIDs := make([]string, 0)
	for _, connection := range connections {
		if !connectors.DiscordEventMatches(connection, event) {
			continue
		}
		notificationID, created, enqueueErr := api.enqueueConnectorEvent(
			request.Context(), connection, normalized,
		)
		if enqueueErr != nil {
			api.storeError(writer, enqueueErr)
			return
		}
		if created {
			accepted++
			notificationIDs = append(notificationIDs, notificationID)
		}
	}
	writeJSON(writer, http.StatusAccepted, map[string]any{
		"accepted": accepted, "notification_ids": notificationIDs,
	})
}
