package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"dispatch/internal/ai"
	"dispatch/internal/config"
	"dispatch/internal/delivery"
	"dispatch/internal/jobs"
	"dispatch/internal/notification"
	"dispatch/internal/routing"
	"dispatch/internal/store/postgres"
	"dispatch/internal/template"
)

type Worker struct {
	Config    config.Config
	Store     *postgres.Store
	AI        ai.DecisionProvider
	Providers map[delivery.Channel]delivery.Provider
	Logger    *slog.Logger
}

func (worker *Worker) Run(ctx context.Context) error {
	if worker.Logger == nil {
		worker.Logger = slog.Default()
	}
	recovered, err := worker.Store.RecoverStaleJobs(ctx, worker.Config.JobLockTimeout)
	if err != nil {
		return fmt.Errorf("recover stale jobs: %w", err)
	}
	worker.Logger.Info("worker started", "id", worker.Config.WorkerID, "concurrency", worker.Config.WorkerConcurrency, "recovered_jobs", recovered)
	var group sync.WaitGroup
	if worker.Config.Telegram.Token != "" {
		group.Add(1)
		go func() {
			defer group.Done()
			worker.telegramDeliveryPairingLoop(ctx)
		}()
	}
	if worker.Config.Connectors.TelegramAPIID > 0 &&
		worker.Config.Connectors.TelegramAPIHash != "" {
		group.Add(1)
		go func() {
			defer group.Done()
			worker.telegramAccountLoop(ctx)
		}()
	}
	if worker.Config.Connectors.GoogleClientID != "" &&
		worker.Config.Connectors.GoogleClientSecret != "" {
		group.Add(1)
		go func() {
			defer group.Done()
			worker.googleLoop(ctx)
		}()
		group.Add(1)
		go func() {
			defer group.Done()
			worker.youtubeLoop(ctx)
		}()
	}
	for index := 0; index < worker.Config.WorkerConcurrency; index++ {
		group.Add(1)
		go func(slot int) {
			defer group.Done()
			worker.loop(ctx, fmt.Sprintf("%s-%d", worker.Config.WorkerID, slot+1))
		}(index)
	}
	group.Wait()
	return nil
}

func (worker *Worker) loop(ctx context.Context, workerID string) {
	ticker := time.NewTicker(worker.Config.JobPollInterval)
	defer ticker.Stop()
	for {
		job, err := worker.Store.ClaimJob(ctx, workerID)
		if err == nil {
			worker.handle(ctx, job)
			continue
		}
		if !errors.Is(err, postgres.ErrNotFound) && !errors.Is(err, context.Canceled) {
			worker.Logger.Error("claim job", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (worker *Worker) handle(ctx context.Context, job jobs.Job) {
	var err error
	switch job.Type {
	case "process_notification":
		err = worker.processNotification(ctx, payload(job, "notification_id"))
	case "deliver":
		err = worker.deliver(ctx, job, payload(job, "delivery_id"))
	default:
		err = fmt.Errorf("unsupported job type %q", job.Type)
	}
	if err == nil {
		if finishErr := worker.Store.CompleteJob(ctx, job.ID); finishErr != nil {
			worker.Logger.Error("complete job", "job_id", job.ID, "error", finishErr)
		}
		return
	}
	delay := jobs.RetryDelay(job.AttemptCount)
	exhausted, retryErr := worker.Store.RetryJob(ctx, job, err, delay)
	worker.Logger.Warn("job failed", "job_id", job.ID, "type", job.Type, "attempt", job.AttemptCount, "exhausted", exhausted, "error", err)
	if retryErr != nil {
		worker.Logger.Error("schedule retry", "job_id", job.ID, "error", retryErr)
	}
}

func (worker *Worker) processNotification(ctx context.Context, notificationID string) error {
	item, err := worker.Store.GetNotification(ctx, notificationID)
	if err != nil {
		return err
	}
	if item.Status == notification.StatusCancelled || item.Status == notification.StatusDelivered {
		return nil
	}
	if item.Status == notification.StatusReceived {
		if err := worker.Store.TransitionNotification(ctx, item.ID, notification.StatusAnalyzing); err != nil {
			return err
		}
	}
	person, err := worker.Store.GetRecipient(ctx, item.RecipientID)
	if err != nil {
		return err
	}
	var decision *ai.Decision
	record := ai.Record{
		ID: "aid_" + uuid.NewString(), NotificationID: item.ID, Provider: "fallback",
		Model: "", PromptVersion: worker.Config.AI.PromptVersion, Status: ai.StatusSkipped, CreatedAt: time.Now().UTC(),
		Decision:    ai.Decision{ReasonCodes: []string{}},
		RawResponse: map[string]any{},
	}
	if worker.Config.AI.Enabled && worker.AI != nil {
		started := time.Now()
		aiCtx, cancel := context.WithTimeout(ctx, worker.Config.AI.Timeout)
		value, raw, aiErr := worker.AI.Decide(aiCtx, ai.InputFromNotification(item))
		cancel()
		record.Provider, record.Model, record.RawResponse = worker.AI.Name(), worker.AI.Model(), raw
		record.DurationMS = time.Since(started).Milliseconds()
		value = ai.NormalizeDecision(value)
		if aiErr == nil && ai.ValidateDecisionWithSummaryLimit(value, worker.Config.AI.SummaryMaxChars) {
			decision, record.Decision, record.Status = &value, value, ai.StatusCompleted
		} else {
			record.Status = ai.StatusFallbackUsed
			if aiErr != nil {
				record.FallbackReason = ai.FallbackReason(aiErr)
			} else {
				record.FallbackReason = "invalid_output"
			}
		}
	} else {
		record.Status, record.FallbackReason = ai.StatusFallbackUsed, "disabled_or_unconfigured"
	}
	final := routing.Decide(routing.PolicyInput{
		Notification: item, Recipient: person, AI: decision,
		AIConfigured: worker.Config.AI.Enabled && worker.AI != nil, Now: time.Now().UTC(),
	}, worker.Config.AI.MinConfidence, worker.providerAvailability())
	if len(final.Channels) == 0 {
		record.Status, record.FallbackReason = ai.StatusFallbackUsed, "no_available_destination"
		_ = worker.Store.SaveAIDecision(ctx, record)
		_ = worker.Store.TransitionNotification(ctx, item.ID, notification.StatusFailed)
		return nil
	}
	if final.FallbackReason != "" && !final.UsedAI &&
		(record.FallbackReason == "" || decision != nil) {
		record.Status, record.FallbackReason = ai.StatusFallbackUsed, final.FallbackReason
	}
	if err := worker.Store.SaveAIDecision(ctx, record); err != nil {
		return err
	}
	if err := worker.Store.SetNotificationAnalysis(ctx, item.ID, final.Category, final.Priority, final.Summary); err != nil {
		return err
	}
	destinations := map[string]string{"email": person.Email, "telegram": person.TelegramChatID, "webhook": person.WebhookURL}
	_, err = worker.Store.CreateDeliveries(ctx, item.ID, final.Channels, destinations, final.SendAt)
	return err
}

func (worker *Worker) deliver(ctx context.Context, job jobs.Job, deliveryID string) error {
	value, err := worker.Store.GetDeliveryContext(ctx, deliveryID)
	if err != nil {
		return err
	}
	if value.Notification.Status == notification.StatusCancelled || value.Delivery.Status == delivery.StatusDelivered {
		return nil
	}
	provider := worker.Providers[value.Delivery.Channel]
	if provider == nil || !provider.Configured() {
		return fmt.Errorf("provider %s is not configured", value.Delivery.Channel)
	}
	attempt, err := worker.Store.BeginDeliveryAttempt(ctx, value.Delivery.ID, fmt.Sprintf("%T", provider), value.Delivery.AttemptCount+1)
	if err != nil {
		return err
	}
	providerCtx, cancel := context.WithTimeout(ctx, worker.Config.ProviderTimeout)
	subject, body := value.Notification.Subject, value.Notification.Body
	messageTemplate, templateErr := worker.Store.MatchTemplate(
		ctx, value.Notification, string(value.Delivery.Channel),
	)
	if templateErr != nil {
		cancel()
		return templateErr
	}
	if messageTemplate != nil {
		subject, body = template.Render(*messageTemplate, value.Notification)
	}
	result, deliveryErr := provider.Deliver(providerCtx, delivery.Message{
		DeliveryID: value.Delivery.ID, NotificationID: value.Notification.ID,
		Destination: value.Delivery.Destination, Subject: subject,
		Body: body, Metadata: value.Notification.Metadata,
	})
	cancel()
	if deliveryErr == nil {
		return worker.Store.FinishDeliverySuccess(ctx, attempt, result.ResponseCode)
	}
	code, retryable := "provider_error", true
	var providerError *delivery.ProviderError
	if errors.As(deliveryErr, &providerError) {
		code, retryable = providerError.Code, providerError.Retryable
	}
	deadLetter := !retryable || job.AttemptCount >= job.MaxAttempts
	if err := worker.Store.FinishDeliveryFailure(ctx, attempt, code, deliveryErr.Error(), time.Now().UTC().Add(jobs.RetryDelay(job.AttemptCount)), deadLetter); err != nil {
		return errors.Join(deliveryErr, err)
	}
	if deadLetter {
		return nil
	}
	return deliveryErr
}

func (worker *Worker) providerAvailability() map[string]bool {
	result := map[string]bool{}
	for channel, provider := range worker.Providers {
		result[string(channel)] = provider != nil && provider.Configured()
	}
	return result
}

func payload(job jobs.Job, key string) string {
	if value, ok := job.Payload[key].(string); ok {
		return value
	}
	return fmt.Sprint(job.Payload[key])
}
