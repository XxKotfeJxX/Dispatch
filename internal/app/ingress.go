package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"dispatch/internal/ingress"
	"dispatch/internal/notification"
	"dispatch/internal/store/postgres"
)

var sourceSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)

func (api *API) createIngressSource(writer http.ResponseWriter, request *http.Request) {
	var input ingress.CreateInput
	if !decode(writer, request, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.ToLower(strings.TrimSpace(input.Slug))
	ingress.ApplyPreset(&input)
	if input.Name == "" || !sourceSlugPattern.MatchString(input.Slug) || input.RecipientID == "" {
		writeError(writer, http.StatusBadRequest, "validation_error",
			"name, recipient_id and a 3-63 character lowercase slug are required")
		return
	}
	if !validChannels(input.Mapping.RequestedChannels) {
		writeError(writer, http.StatusBadRequest, "validation_error", "requested channels are invalid")
		return
	}
	if input.AuthMode == ingress.AuthHeader && input.AuthHeader == "" {
		input.AuthHeader = "X-Webhook-Secret"
	}
	secret := strings.TrimSpace(input.Secret)
	generated := secret == ""
	if generated {
		if input.AuthMode == ingress.AuthSlack || input.AuthMode == ingress.AuthStripe {
			writeError(writer, http.StatusBadRequest, "validation_error",
				"the provider signing secret is required for Slack and Stripe")
			return
		}
		var err error
		secret, err = ingress.GenerateSecret()
		if err != nil {
			api.storeError(writer, err)
			return
		}
	}
	var hash, cipher []byte
	var err error
	switch input.AuthMode {
	case ingress.AuthBearer, ingress.AuthHeader:
		hash = ingress.HashSecret(secret)
	case ingress.AuthHMAC, ingress.AuthSlack, ingress.AuthStripe:
		cipher, err = ingress.EncryptSecret(api.Config.Ingress.EncryptionKey, secret)
	default:
		writeError(writer, http.StatusBadRequest, "validation_error", "unsupported auth mode")
		return
	}
	if err != nil {
		api.storeError(writer, err)
		return
	}
	item, err := api.Store.CreateIngressSource(request.Context(), input, hash, cipher)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	item.SignatureHeader = ingress.SignatureHeader(item)
	returnedSecret := ""
	warning := "The supplied signing secret was encrypted and is not returned."
	if generated {
		returnedSecret = secret
		warning = "The secret is shown once and cannot be recovered."
	}
	writeJSON(writer, http.StatusCreated, map[string]any{
		"data":     ingress.Created{Source: item, Secret: returnedSecret},
		"endpoint": "/ingest/v1/" + item.Slug,
		"warning":  warning,
	})
}

func (api *API) listIngressSources(writer http.ResponseWriter, request *http.Request) {
	items, err := api.Store.ListIngressSources(request.Context())
	if err != nil {
		api.storeError(writer, err)
		return
	}
	for index := range items {
		items[index].SignatureHeader = ingress.SignatureHeader(items[index])
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": items, "providers": ingress.Providers(),
	})
}

func (api *API) rotateIngressSecret(writer http.ResponseWriter, request *http.Request) {
	id := chi.URLParam(request, "id")
	var input struct {
		Secret string `json:"secret"`
	}
	if !decode(writer, request, &input) {
		return
	}
	items, err := api.Store.ListIngressSources(request.Context())
	if err != nil {
		api.storeError(writer, err)
		return
	}
	var source *ingress.Source
	for index := range items {
		if items[index].ID == id {
			source = &items[index]
			break
		}
	}
	if source == nil {
		api.storeError(writer, postgres.ErrNotFound)
		return
	}
	secret := strings.TrimSpace(input.Secret)
	generated := secret == ""
	if generated {
		if source.AuthMode == ingress.AuthSlack || source.AuthMode == ingress.AuthStripe {
			writeError(writer, http.StatusBadRequest, "validation_error",
				"the new provider signing secret is required for Slack and Stripe")
			return
		}
		secret, err = ingress.GenerateSecret()
		if err != nil {
			api.storeError(writer, err)
			return
		}
	}
	var hash, cipher []byte
	if source.AuthMode == ingress.AuthBearer || source.AuthMode == ingress.AuthHeader {
		hash = ingress.HashSecret(secret)
	} else {
		cipher, err = ingress.EncryptSecret(api.Config.Ingress.EncryptionKey, secret)
	}
	if err != nil {
		api.storeError(writer, err)
		return
	}
	if err := api.Store.RotateIngressSecret(request.Context(), id, hash, cipher); err != nil {
		api.storeError(writer, err)
		return
	}
	returnedSecret := ""
	warning := "The previous secret is invalid. The supplied signing secret was encrypted."
	if generated {
		returnedSecret = secret
		warning = "The previous secret is invalid. This secret is shown once."
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"data": map[string]string{"secret": returnedSecret}, "warning": warning,
	})
}

func (api *API) setIngressSourceEnabled(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(writer, request, &input) {
		return
	}
	if err := api.Store.SetIngressSourceEnabled(
		request.Context(), chi.URLParam(request, "id"), input.Enabled,
	); err != nil {
		api.storeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) deleteIngressSource(writer http.ResponseWriter, request *http.Request) {
	if err := api.Store.DeleteIngressSource(request.Context(), chi.URLParam(request, "id")); err != nil {
		api.storeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) listIngressEvents(writer http.ResponseWriter, request *http.Request) {
	events, err := api.Store.IngressEvents(request.Context(), chi.URLParam(request, "id"), 100)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": events})
}

func (api *API) receiveIngress(writer http.ResponseWriter, request *http.Request) {
	source, hash, cipher, err := api.Store.GetIngressSourceBySlug(
		request.Context(), chi.URLParam(request, "slug"),
	)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	if !source.Enabled {
		writeError(writer, http.StatusGone, "source_disabled", "ingress source is disabled")
		return
	}
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_body", "could not read request body")
		return
	}
	if len(raw) == 0 {
		writeError(writer, http.StatusBadRequest, "invalid_body", "request body is empty")
		return
	}
	if !api.verifyIngressRequest(source, hash, cipher, raw, request) {
		writeError(writer, http.StatusUnauthorized, "invalid_source_signature", "source authentication failed")
		return
	}
	if source.Provider == "slack" {
		var challenge struct {
			Type      string `json:"type"`
			Challenge string `json:"challenge"`
		}
		if json.Unmarshal(raw, &challenge) == nil &&
			challenge.Type == "url_verification" && challenge.Challenge != "" {
			writeJSON(writer, http.StatusOK, map[string]string{"challenge": challenge.Challenge})
			return
		}
	}
	headers := make(map[string]string)
	for key, values := range request.Header {
		if len(values) > 0 {
			headers[strings.ToLower(key)] = values[0]
		}
	}
	transformed, err := ingress.Transform(source, raw, headers)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	digest := sha256.Sum256(raw)
	payloadDigest := hex.EncodeToString(digest[:])
	if transformed.ExternalID == "" {
		transformed.ExternalID = payloadDigest
	}
	idempotencyDigest := sha256.Sum256([]byte(source.ID + ":" + transformed.ExternalID))
	item := notification.Notification{
		IdempotencyKey: "ing_" + hex.EncodeToString(idempotencyDigest[:]),
		RecipientID:    source.RecipientID, EventType: transformed.EventType,
		Subject: transformed.Subject, Body: transformed.Body,
		Metadata: transformed.Metadata, RequestedChannels: source.Mapping.RequestedChannels,
	}
	created, err := api.Store.CreateNotification(request.Context(), &item)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	if err := api.Store.RecordIngressEvent(request.Context(), ingress.Event{
		SourceID: source.ID, ExternalID: transformed.ExternalID,
		NotificationID: item.ID, EventType: item.EventType,
		PayloadDigest: payloadDigest, ReceivedAt: time.Now().UTC(),
	}); err != nil {
		api.storeError(writer, err)
		return
	}
	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	writeJSON(writer, status, map[string]any{
		"data": map[string]any{"notification_id": item.ID, "created": created, "status": item.Status},
	})
}

func (api *API) verifyIngressRequest(
	source ingress.Source,
	hash, cipher, raw []byte,
	request *http.Request,
) bool {
	switch source.AuthMode {
	case ingress.AuthBearer:
		value := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
		if value == "" {
			value = request.Header.Get("X-Dispatch-Ingest-Key")
		}
		return ingress.VerifyStoredHash(hash, value)
	case ingress.AuthHeader:
		return ingress.VerifyStoredHash(hash, request.Header.Get(source.AuthHeader))
	case ingress.AuthHMAC:
		secret, err := ingress.DecryptSecret(api.Config.Ingress.EncryptionKey, cipher)
		return err == nil && ingress.VerifyHMAC(secret, raw, request.Header.Get(ingress.SignatureHeader(source)))
	case ingress.AuthSlack:
		secret, err := ingress.DecryptSecret(api.Config.Ingress.EncryptionKey, cipher)
		return err == nil && ingress.VerifySlack(secret, raw,
			request.Header.Get("X-Slack-Request-Timestamp"),
			request.Header.Get("X-Slack-Signature"), time.Now())
	case ingress.AuthStripe:
		secret, err := ingress.DecryptSecret(api.Config.Ingress.EncryptionKey, cipher)
		return err == nil && ingress.VerifyStripe(secret, raw,
			request.Header.Get("Stripe-Signature"), time.Now())
	default:
		return false
	}
}
