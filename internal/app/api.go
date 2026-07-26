package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"dispatch/internal/ai"
	"dispatch/internal/buildinfo"
	"dispatch/internal/config"
	"dispatch/internal/notification"
	"dispatch/internal/recipient"
	"dispatch/internal/routing"
	"dispatch/internal/store/postgres"
	"dispatch/internal/template"
)

type API struct {
	Config   config.Config
	Store    *postgres.Store
	AI       ai.DecisionProvider
	Logger   *slog.Logger
	requests atomic.Int64
	errors   atomic.Int64
	limiter  *ipLimiter
}

func (api *API) Handler() http.Handler {
	if api.Logger == nil {
		api.Logger = slog.Default()
	}
	api.limiter = &ipLimiter{entries: map[string]*limitEntry{}, maximum: api.Config.RateLimitPerMin}
	router := chi.NewRouter()
	router.Use(api.recoverer, api.cors, api.observe, api.limitBody)
	router.Get("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Get("/readyz", api.ready)
	router.Get("/metrics", api.metrics)
	router.Post("/internal/v1/discord/events", api.discordBridgeEvent)
	router.With(api.rateLimit).Post("/ingest/v1/{slug}", api.receiveIngress)
	router.With(api.rateLimit).Get("/connect/v1/oauth/{connector}/callback", api.connectorOAuthCallback)
	router.With(api.rateLimit).Get("/connect/v1/github/setup", api.completeGitHubInstall)
	router.Post("/connect/v1/github/events", api.githubWebhook)
	router.Group(func(protected chi.Router) {
		protected.Use(api.authenticate, api.rateLimit)
		protected.Route("/api/v1", func(routes chi.Router) {
			routes.Get("/dashboard", api.dashboard)
			routes.Get("/settings", api.settings)
			routes.Get("/events", api.events)
			routes.Get("/notifications", api.listNotifications)
			routes.Post("/notifications", api.createNotification)
			routes.Get("/notifications/{id}", api.getNotification)
			routes.Post("/notifications/{id}/cancel", api.cancelNotification)
			routes.Post("/notifications/{id}/retry", api.retryNotification)
			routes.Get("/recipients", api.listRecipients)
			routes.Post("/recipients", api.createRecipient)
			routes.Get("/recipients/{id}", api.getRecipient)
			routes.Put("/recipients/{id}", api.updateRecipient)
			routes.Delete("/recipients/{id}", api.deleteRecipient)
			routes.Post("/recipient-setups/mailpit", api.startMailpitRecipientSetup)
			routes.Post("/recipient-setups/mailpit/{id}/verify", api.verifyMailpitRecipientSetup)
			routes.Post("/recipient-setups/telegram", api.startTelegramRecipientSetup)
			routes.Get("/recipient-setups/{id}", api.getRecipientSetup)
			routes.Get("/templates", api.listTemplates)
			routes.Post("/templates", api.createTemplate)
			routes.Put("/templates/{id}", api.updateTemplate)
			routes.Delete("/templates/{id}", api.deleteTemplate)
			routes.Get("/rules", api.listRules)
			routes.Post("/rules", api.createRule)
			routes.Put("/rules/{id}", api.updateRule)
			routes.Delete("/rules/{id}", api.deleteRule)
			routes.Post("/ai/preview", api.aiPreview)
			routes.Get("/sources", api.listIngressSources)
			routes.Post("/sources", api.createIngressSource)
			routes.Post("/sources/{id}/rotate-secret", api.rotateIngressSecret)
			routes.Post("/sources/{id}/enabled", api.setIngressSourceEnabled)
			routes.Get("/sources/{id}/events", api.listIngressEvents)
			routes.Delete("/sources/{id}", api.deleteIngressSource)
			routes.Get("/connectors", api.listConnectors)
			routes.Post("/connectors/{connector}/authorize", api.beginConnectorOAuth)
			routes.Post("/connectors/github/install", api.beginGitHubInstall)
			routes.Post("/connectors/telegram/auth/start", api.beginTelegramAccountAuth)
			routes.Post("/connectors/telegram/auth/{authID}/code", api.completeTelegramAccountCode)
			routes.Post("/connectors/telegram/auth/{authID}/password", api.completeTelegramAccountPassword)
			routes.Post("/connections", api.createConnectorConnection)
			routes.Post("/connections/{id}/test", api.testConnectorConnection)
			routes.Post("/connections/{id}/sample", api.connectorSample)
			routes.Post("/connections/{id}/enabled", api.setConnectorEnabled)
			routes.Delete("/connections/{id}", api.deleteConnectorConnection)
		})
	})
	return router
}

func (api *API) createNotification(writer http.ResponseWriter, request *http.Request) {
	var input notification.CreateInput
	if !decode(writer, request, &input) {
		return
	}
	if input.IdempotencyKey == "" || input.RecipientID == "" || input.EventType == "" || input.Body == "" {
		writeError(writer, http.StatusBadRequest, "validation_error", "idempotency_key, recipient_id, event_type and body are required")
		return
	}
	if len(input.IdempotencyKey) > 200 || len(input.Body) > 100_000 || !validChannels(input.RequestedChannels) {
		writeError(writer, http.StatusBadRequest, "validation_error", "request fields are outside allowed limits")
		return
	}
	item := notification.Notification{
		IdempotencyKey: input.IdempotencyKey, RecipientID: input.RecipientID,
		EventType: input.EventType, Subject: input.Subject, Body: input.Body,
		Metadata: input.Metadata, RequestedChannels: input.RequestedChannels, ScheduledAt: input.ScheduledAt,
	}
	created, err := api.Store.CreateNotification(request.Context(), &item)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	writer.Header().Set("Location", "/api/v1/notifications/"+item.ID)
	writeJSON(writer, status, map[string]any{"data": item, "created": created})
}

func (api *API) listNotifications(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	filter := notification.Filter{Status: query.Get("status"), Priority: query.Get("priority"), Channel: query.Get("channel"), EventType: query.Get("event_type")}
	filter.Limit, _ = strconv.Atoi(query.Get("limit"))
	filter.Offset, _ = strconv.Atoi(query.Get("offset"))
	if value := query.Get("ai_fallback"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			filter.AIFallback = &parsed
		}
	}
	items, total, err := api.Store.ListNotifications(request.Context(), filter)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": items, "total": total, "limit": filter.Limit, "offset": filter.Offset})
}

func (api *API) getNotification(writer http.ResponseWriter, request *http.Request) {
	id := chi.URLParam(request, "id")
	item, err := api.Store.GetNotification(request.Context(), id)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	deliveries, err := api.Store.Deliveries(request.Context(), id)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	attempts, err := api.Store.DeliveryAttempts(request.Context(), id)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	decisions, err := api.Store.AIDecisions(request.Context(), id)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	events, err := api.Store.AuditEvents(request.Context(), "notification", id)
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": item, "deliveries": deliveries, "attempts": attempts, "ai_decisions": decisions, "events": events})
}

func (api *API) cancelNotification(writer http.ResponseWriter, request *http.Request) {
	if err := api.Store.CancelNotification(request.Context(), chi.URLParam(request, "id")); err != nil {
		api.storeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
func (api *API) retryNotification(writer http.ResponseWriter, request *http.Request) {
	if err := api.Store.RetryNotification(request.Context(), chi.URLParam(request, "id")); err != nil {
		api.storeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusAccepted)
}

func (api *API) listRecipients(writer http.ResponseWriter, request *http.Request) {
	items, err := api.Store.ListRecipients(request.Context())
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": items})
}
func (api *API) getRecipient(writer http.ResponseWriter, request *http.Request) {
	item, err := api.Store.GetRecipient(request.Context(), chi.URLParam(request, "id"))
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": item})
}
func (api *API) createRecipient(writer http.ResponseWriter, request *http.Request) {
	var item recipient.Recipient
	if !decode(writer, request, &item) {
		return
	}
	normalizeRecipient(&item)
	if err := validateRecipient(item); err != nil {
		writeError(writer, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if err := api.Store.CreateRecipient(request.Context(), &item); err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"data": item})
}
func (api *API) updateRecipient(writer http.ResponseWriter, request *http.Request) {
	var item recipient.Recipient
	if !decode(writer, request, &item) {
		return
	}
	item.ID = chi.URLParam(request, "id")
	normalizeRecipient(&item)
	if err := validateRecipient(item); err != nil {
		writeError(writer, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if err := api.Store.UpdateRecipient(request.Context(), item); err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": item})
}
func (api *API) deleteRecipient(writer http.ResponseWriter, request *http.Request) {
	if err := api.Store.DeleteRecipient(request.Context(), chi.URLParam(request, "id")); err != nil {
		api.storeError(writer, err)
		return
	}
	writer.WriteHeader(204)
}

func (api *API) listTemplates(writer http.ResponseWriter, request *http.Request) {
	items, err := api.Store.ListTemplates(request.Context())
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, 200, map[string]any{"data": items})
}
func (api *API) createTemplate(writer http.ResponseWriter, request *http.Request) {
	var item template.Template
	if !decode(writer, request, &item) {
		return
	}
	normalizeTemplate(&item)
	item.Enabled = true
	if err := template.Validate(item); err != nil {
		writeError(writer, 400, "validation_error", err.Error())
		return
	}
	if err := api.Store.CreateTemplate(request.Context(), &item); err != nil {
		if errors.Is(err, postgres.ErrConflict) {
			writeError(writer, 409, "fallback_exists",
				"this service already has a fallback template; add a condition or edit the existing fallback")
			return
		}
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, 201, map[string]any{"data": item})
}
func (api *API) updateTemplate(writer http.ResponseWriter, request *http.Request) {
	var item template.Template
	if !decode(writer, request, &item) {
		return
	}
	item.ID = chi.URLParam(request, "id")
	normalizeTemplate(&item)
	if err := template.Validate(item); err != nil {
		writeError(writer, 400, "validation_error", err.Error())
		return
	}
	if err := api.Store.UpdateTemplate(request.Context(), item); err != nil {
		if errors.Is(err, postgres.ErrConflict) {
			writeError(writer, 409, "fallback_exists",
				"this service already has a fallback template; add a condition or edit the existing fallback")
			return
		}
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, 200, map[string]any{"data": item})
}
func (api *API) deleteTemplate(writer http.ResponseWriter, request *http.Request) {
	if err := api.Store.DeleteTemplate(request.Context(), chi.URLParam(request, "id")); err != nil {
		api.storeError(writer, err)
		return
	}
	writer.WriteHeader(204)
}

func normalizeTemplate(item *template.Template) {
	item.Name = strings.TrimSpace(item.Name)
	item.Service = strings.ToLower(strings.TrimSpace(item.Service))
	item.Channel = strings.ToLower(strings.TrimSpace(item.Channel))
	if item.Service == "" {
		item.Service = template.ServiceAny
	}
	if item.Channel == "" {
		item.Channel = template.ChannelAll
	}
	for index := range item.Conditions {
		item.Conditions[index].Field = strings.ToLower(strings.TrimSpace(item.Conditions[index].Field))
		item.Conditions[index].Operator = strings.ToLower(strings.TrimSpace(item.Conditions[index].Operator))
		item.Conditions[index].Value = strings.TrimSpace(item.Conditions[index].Value)
	}
}

func (api *API) listRules(writer http.ResponseWriter, request *http.Request) {
	items, err := api.Store.ListRules(request.Context())
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, 200, map[string]any{"data": items})
}
func (api *API) createRule(writer http.ResponseWriter, request *http.Request) {
	var item routing.Rule
	if !decode(writer, request, &item) {
		return
	}
	if item.Name == "" || item.Condition == nil || item.Action == nil {
		writeError(writer, 400, "validation_error", "name, condition and action are required")
		return
	}
	if err := api.Store.CreateRule(request.Context(), &item); err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, 201, map[string]any{"data": item})
}
func (api *API) updateRule(writer http.ResponseWriter, request *http.Request) {
	var item routing.Rule
	if !decode(writer, request, &item) {
		return
	}
	item.ID = chi.URLParam(request, "id")
	if err := api.Store.UpdateRule(request.Context(), item); err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, 200, map[string]any{"data": item})
}
func (api *API) deleteRule(writer http.ResponseWriter, request *http.Request) {
	if err := api.Store.DeleteRule(request.Context(), chi.URLParam(request, "id")); err != nil {
		api.storeError(writer, err)
		return
	}
	writer.WriteHeader(204)
}

func (api *API) dashboard(writer http.ResponseWriter, request *http.Request) {
	result, err := api.Store.Dashboard(request.Context())
	if err != nil {
		api.storeError(writer, err)
		return
	}
	writeJSON(writer, 200, map[string]any{"data": result})
}
func (api *API) settings(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, 200, map[string]any{"data": map[string]any{
		"version":  buildinfo.Version,
		"auth":     map[string]bool{"enabled": api.Config.ConsoleAuthEnabled},
		"ai":       map[string]any{"enabled": api.Config.AI.Enabled, "model": api.Config.AI.Model, "prompt_version": api.Config.AI.PromptVersion},
		"channels": map[string]bool{"email": api.Config.SMTP.Host != "", "telegram": api.Config.Telegram.Token != "", "webhook": api.Config.Webhook.Enabled},
		"connectors": map[string]bool{
			"public_https": strings.HasPrefix(
				strings.ToLower(api.Config.Connectors.PublicURL), "https://",
			),
			"github_app": api.Config.Connectors.GitHubAppSlug != "" &&
				api.Config.Connectors.GitHubAppID > 0 &&
				api.Config.Connectors.GitHubWebhookSecret != "" &&
				api.Config.Connectors.GitHubPrivateKeyB64 != "",
			"discord_app": api.Config.Connectors.DiscordClientID != "" &&
				api.Config.Connectors.DiscordClientSecret != "" &&
				api.Config.Connectors.DiscordBotToken != "",
			"telegram_account": api.Config.Connectors.TelegramAPIID > 0 &&
				api.Config.Connectors.TelegramAPIHash != "",
			"google_oauth": api.Config.Connectors.GoogleClientID != "" &&
				api.Config.Connectors.GoogleClientSecret != "",
		},
		"worker_concurrency": api.Config.WorkerConcurrency,
	}})
}
func (api *API) aiPreview(writer http.ResponseWriter, request *http.Request) {
	if api.AI == nil || !api.Config.AI.Enabled {
		writeError(writer, 503, "ai_unavailable", "AI routing is disabled")
		return
	}
	var input ai.DecisionInput
	if !decode(writer, request, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), api.Config.AI.Timeout)
	defer cancel()
	decision, _, err := api.AI.Decide(ctx, input)
	if err != nil {
		writeError(writer, 502, "ai_provider_error", "AI preview failed")
		return
	}
	writeJSON(writer, 200, map[string]any{"data": decision})
}

func (api *API) events(writer http.ResponseWriter, request *http.Request) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeError(writer, 500, "streaming_unsupported", "streaming is unavailable")
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	after, _ := strconv.ParseInt(request.URL.Query().Get("after"), 10, 64)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		events, err := api.Store.AuditEventsAfter(request.Context(), after, 100)
		if err != nil {
			return
		}
		for _, event := range events {
			raw, _ := json.Marshal(event)
			fmt.Fprintf(writer, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.EventType, raw)
			after = event.ID
		}
		flusher.Flush()
		select {
		case <-request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (api *API) ready(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if err := api.Store.Ready(ctx); err != nil {
		writeError(writer, 503, "database_unavailable", "database is unavailable")
		return
	}
	writeJSON(writer, 200, map[string]string{"status": "ready"})
}
func (api *API) metrics(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(writer, "# TYPE dispatch_http_requests_total counter\ndispatch_http_requests_total %d\n# TYPE dispatch_http_errors_total counter\ndispatch_http_errors_total %d\n", api.requests.Load(), api.errors.Load())
}

func (api *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !api.Config.ConsoleAuthEnabled {
			next.ServeHTTP(writer, request)
			return
		}
		provided := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
		if provided == "" {
			provided = request.Header.Get("X-API-Key")
		}
		expected := api.Config.APIKey
		if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			writeError(writer, 401, "unauthorized", "a valid API key is required")
			return
		}
		next.ServeHTTP(writer, request)
	})
}
func (api *API) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Origin") == api.Config.WebOrigin {
			writer.Header().Set("Access-Control-Allow-Origin", api.Config.WebOrigin)
			writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, Idempotency-Key")
			writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			writer.Header().Set("Vary", "Origin")
		}
		if request.Method == http.MethodOptions {
			writer.WriteHeader(204)
			return
		}
		next.ServeHTTP(writer, request)
	})
}
func (api *API) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(writer, request.Body, api.Config.MaxRequestBytes)
		next.ServeHTTP(writer, request)
	})
}
func (api *API) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		api.requests.Add(1)
		next.ServeHTTP(writer, request)
	})
}
func (api *API) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				api.errors.Add(1)
				api.Logger.Error("request panic", "path", request.URL.Path)
				writeError(writer, 500, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(writer, request)
	})
}
func (api *API) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		host, _, _ := net.SplitHostPort(request.RemoteAddr)
		if !api.limiter.allow(host, time.Now()) {
			writer.Header().Set("Retry-After", "60")
			writeError(writer, 429, "rate_limited", "too many requests")
			return
		}
		next.ServeHTTP(writer, request)
	})
}
func (api *API) storeError(writer http.ResponseWriter, err error) {
	api.errors.Add(1)
	switch {
	case errors.Is(err, postgres.ErrNotFound):
		writeError(writer, 404, "not_found", "resource not found")
	case errors.Is(err, postgres.ErrConflict):
		writeError(writer, 409, "conflict", "resource already exists")
	case errors.Is(err, postgres.ErrInvalidState):
		writeError(writer, 409, "invalid_state", "operation is not valid for the current state")
	default:
		api.Logger.Error("store operation", "error", err)
		writeError(writer, 500, "internal_error", "internal server error")
	}
}

type limitEntry struct {
	started time.Time
	count   int
}
type ipLimiter struct {
	sync.Mutex
	entries map[string]*limitEntry
	maximum int
}

func (limiter *ipLimiter) allow(key string, now time.Time) bool {
	limiter.Lock()
	defer limiter.Unlock()
	entry := limiter.entries[key]
	if entry == nil || now.Sub(entry.started) >= time.Minute {
		limiter.entries[key] = &limitEntry{started: now, count: 1}
		return true
	}
	if entry.count >= limiter.maximum {
		return false
	}
	entry.count++
	return true
}
func decode(writer http.ResponseWriter, request *http.Request, target any) bool {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(writer, 400, "invalid_json", "request body is invalid")
		return false
	}
	return true
}
func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func validChannels(channels []string) bool {
	for _, channel := range channels {
		if channel != "email" && channel != "telegram" && channel != "webhook" {
			return false
		}
	}
	return true
}
func normalizeRecipient(item *recipient.Recipient) {
	item.Name = strings.TrimSpace(item.Name)
	item.DestinationType = strings.ToLower(strings.TrimSpace(item.DestinationType))
	item.DestinationLabel = strings.TrimSpace(item.DestinationLabel)
	item.Email = strings.ToLower(strings.TrimSpace(item.Email))
	item.TelegramChatID = strings.TrimSpace(item.TelegramChatID)
	item.WebhookURL = strings.TrimSpace(item.WebhookURL)
	if item.DestinationType == "" {
		switch {
		case item.TelegramChatID != "":
			item.DestinationType = "telegram"
		case item.WebhookURL != "":
			item.DestinationType = "webhook"
		default:
			item.DestinationType = "email"
		}
	}
	if item.DestinationLabel == "" {
		switch item.DestinationType {
		case "email", "mailpit":
			item.DestinationLabel = item.Email
		case "telegram":
			item.DestinationLabel = "Telegram"
		case "webhook":
			item.DestinationLabel = "Webhook"
		}
	}
}

func validateRecipient(item recipient.Recipient) error {
	if item.Name == "" {
		return fmt.Errorf("recipient name is required")
	}
	if !validChannels(item.Preferences.DefaultChannels) ||
		!validChannels(item.Preferences.DisabledChannels) {
		return fmt.Errorf("unsupported delivery channel")
	}
	if len(item.Preferences.DefaultChannels) == 0 {
		return fmt.Errorf("select a delivery channel")
	}
	hasDefaultChannel := func(expected string) bool {
		for _, channel := range item.Preferences.DefaultChannels {
			if channel == expected {
				return true
			}
		}
		return false
	}
	switch item.DestinationType {
	case "email", "mailpit":
		address, err := mail.ParseAddress(item.Email)
		if err != nil || address.Address != item.Email {
			return fmt.Errorf("enter a valid email address")
		}
		if !hasDefaultChannel("email") {
			return fmt.Errorf("email destination must use the email channel")
		}
	case "telegram":
		if item.TelegramChatID == "" {
			return fmt.Errorf("connect Telegram through the Dispatch bot")
		}
		if !hasDefaultChannel("telegram") {
			return fmt.Errorf("Telegram destination must use the Telegram channel")
		}
	case "webhook":
		target, err := url.ParseRequestURI(item.WebhookURL)
		if err != nil || (target.Scheme != "http" && target.Scheme != "https") ||
			target.Host == "" || target.User != nil {
			return fmt.Errorf("enter a valid HTTP or HTTPS webhook URL")
		}
		if item.DestinationLabel == "" {
			return fmt.Errorf("service name is required")
		}
		if !hasDefaultChannel("webhook") {
			return fmt.Errorf("webhook destination must use the webhook channel")
		}
	default:
		return fmt.Errorf("unsupported destination type")
	}
	return nil
}
