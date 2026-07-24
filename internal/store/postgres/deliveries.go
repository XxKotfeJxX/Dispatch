package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dispatch/internal/delivery"
	"dispatch/internal/notification"
	"dispatch/internal/recipient"
)

type DeliveryContext struct {
	Delivery     delivery.Delivery
	Notification notification.Notification
	Recipient    recipient.Recipient
}

func (store *Store) CreateDeliveries(
	ctx context.Context,
	notificationID string,
	channels []string,
	destinations map[string]string,
	availableAt time.Time,
) ([]delivery.Delivery, error) {
	result := make([]delivery.Delivery, 0, len(channels))
	err := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		for _, channel := range channels {
			destination := destinations[channel]
			if destination == "" {
				continue
			}
			item := delivery.Delivery{
				ID: "del_" + uuid.NewString(), NotificationID: notificationID,
				Channel: delivery.Channel(channel), Destination: destination,
				Status: delivery.StatusQueued, NextAttemptAt: availableAt,
				CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}
			command, err := tx.Exec(ctx, `
				INSERT INTO deliveries
				    (id,notification_id,channel,destination,status,next_attempt_at,created_at,updated_at)
				VALUES ($1,$2,$3,$4,'queued',$5,$6,$6)
				ON CONFLICT(notification_id,channel,destination) DO NOTHING`,
				item.ID, item.NotificationID, item.Channel, item.Destination,
				item.NextAttemptAt, item.CreatedAt)
			if err != nil {
				return err
			}
			if command.RowsAffected() == 0 {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO jobs(id,type,payload_json,status,max_attempts,available_at)
				VALUES ($1,'deliver',jsonb_build_object('delivery_id',$2::text,'notification_id',$3::text),
				        'queued',5,$4)`,
				"job_"+uuid.NewString(), item.ID, notificationID, availableAt); err != nil {
				return err
			}
			result = append(result, item)
		}
		if len(result) == 0 {
			return ErrConflict
		}
		if _, err := tx.Exec(ctx, `
			UPDATE notifications SET status='queued',updated_at=now() WHERE id=$1`, notificationID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_events(entity_type,entity_id,event_type,details_json)
			VALUES ('notification',$1,'notification.deliveries_created',
			        jsonb_build_object('count',$2::int,'channels',$3::text[]))`,
			notificationID, len(result), channels)
		return err
	})
	return result, err
}

func (store *Store) GetDeliveryContext(ctx context.Context, deliveryID string) (DeliveryContext, error) {
	var result DeliveryContext
	var metadata, preferences []byte
	err := store.pool.QueryRow(ctx, `
		SELECT d.id,d.notification_id,d.channel,d.destination,d.status,d.attempt_count,
		       d.next_attempt_at,COALESCE(d.last_error_code,''),COALESCE(d.last_error_message,''),
		       d.created_at,d.updated_at,
		       n.id,n.idempotency_key,n.recipient_id,n.event_type,n.subject,n.body,n.metadata_json,
		       n.requested_channels,n.scheduled_at,COALESCE(n.category,''),n.priority,
		       COALESCE(n.summary,''),n.status,n.created_at,n.updated_at,
		       r.id,r.name,COALESCE(r.email,''),COALESCE(r.telegram_chat_id,''),
		       COALESCE(r.webhook_url,''),r.preferences_json,r.created_at,r.updated_at
		FROM deliveries d
		JOIN notifications n ON n.id=d.notification_id
		JOIN recipients r ON r.id=n.recipient_id
		WHERE d.id=$1`, deliveryID).Scan(
		&result.Delivery.ID, &result.Delivery.NotificationID, &result.Delivery.Channel,
		&result.Delivery.Destination, &result.Delivery.Status, &result.Delivery.AttemptCount,
		&result.Delivery.NextAttemptAt, &result.Delivery.LastErrorCode,
		&result.Delivery.LastErrorMessage, &result.Delivery.CreatedAt, &result.Delivery.UpdatedAt,
		&result.Notification.ID, &result.Notification.IdempotencyKey,
		&result.Notification.RecipientID, &result.Notification.EventType,
		&result.Notification.Subject, &result.Notification.Body, &metadata,
		&result.Notification.RequestedChannels, &result.Notification.ScheduledAt,
		&result.Notification.Category, &result.Notification.Priority,
		&result.Notification.Summary, &result.Notification.Status,
		&result.Notification.CreatedAt, &result.Notification.UpdatedAt,
		&result.Recipient.ID, &result.Recipient.Name, &result.Recipient.Email,
		&result.Recipient.TelegramChatID, &result.Recipient.WebhookURL, &preferences,
		&result.Recipient.CreatedAt, &result.Recipient.UpdatedAt)
	if err != nil {
		return result, notFound(err)
	}
	if err := decodeJSON(metadata, &result.Notification.Metadata); err != nil {
		return result, err
	}
	if err := decodeJSON(preferences, &result.Recipient.Preferences); err != nil {
		return result, err
	}
	return result, nil
}

func (store *Store) BeginDeliveryAttempt(ctx context.Context, deliveryID, provider string, attempt int) (delivery.Attempt, error) {
	item := delivery.Attempt{
		ID: "att_" + uuid.NewString(), DeliveryID: deliveryID, AttemptNumber: attempt,
		Provider: provider, StartedAt: time.Now().UTC(), Status: "sending",
	}
	err := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `
			UPDATE deliveries SET status='sending',attempt_count=$2,updated_at=now()
			WHERE id=$1 AND status IN ('queued','retry_wait')`, deliveryID, attempt)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return ErrInvalidState
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO delivery_attempts
			    (id,delivery_id,attempt_number,provider,started_at,status)
			VALUES ($1,$2,$3,$4,$5,'sending')`,
			item.ID, deliveryID, attempt, provider, item.StartedAt); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE notifications SET status='delivering',updated_at=now()
			WHERE id=(SELECT notification_id FROM deliveries WHERE id=$1)
			  AND status IN ('queued','partially_delivered')`, deliveryID)
		return err
	})
	return item, err
}

func (store *Store) FinishDeliverySuccess(ctx context.Context, attempt delivery.Attempt, responseCode int) error {
	now := time.Now().UTC()
	return pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE delivery_attempts SET status='delivered',finished_at=$2,response_code=$3
			WHERE id=$1`, attempt.ID, now, responseCode); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE deliveries SET status='delivered',last_error_code=NULL,last_error_message=NULL,
			    updated_at=$2 WHERE id=$1`, attempt.DeliveryID, now); err != nil {
			return err
		}
		var notificationID string
		if err := tx.QueryRow(ctx, `SELECT notification_id FROM deliveries WHERE id=$1`, attempt.DeliveryID).Scan(&notificationID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events(entity_type,entity_id,event_type,details_json)
			VALUES ('notification',$1,'delivery.delivered',jsonb_build_object('delivery_id',$2::text))`,
			notificationID, attempt.DeliveryID); err != nil {
			return err
		}
		return refreshNotificationStatus(ctx, tx, notificationID)
	})
}

func (store *Store) FinishDeliveryFailure(
	ctx context.Context,
	attempt delivery.Attempt,
	code, message string,
	nextAttempt time.Time,
	deadLetter bool,
) error {
	now := time.Now().UTC()
	status := delivery.StatusRetryWait
	attemptStatus := "failed"
	if deadLetter {
		status = delivery.StatusDeadLetter
		attemptStatus = "dead_letter"
	}
	return pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE delivery_attempts SET status=$2,finished_at=$3,error_code=$4,error_message=$5
			WHERE id=$1`, attempt.ID, attemptStatus, now, code, message); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE deliveries SET status=$2,next_attempt_at=$3,last_error_code=$4,
			    last_error_message=$5,updated_at=$6 WHERE id=$1`,
			attempt.DeliveryID, status, nextAttempt, code, message, now); err != nil {
			return err
		}
		var notificationID string
		if err := tx.QueryRow(ctx, `SELECT notification_id FROM deliveries WHERE id=$1`, attempt.DeliveryID).Scan(&notificationID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO audit_events(entity_type,entity_id,event_type,details_json)
			VALUES ('notification',$1,$2,jsonb_build_object('delivery_id',$3::text,'error_code',$4::text))`,
			notificationID, "delivery."+string(status), attempt.DeliveryID, code); err != nil {
			return err
		}
		return refreshNotificationStatus(ctx, tx, notificationID)
	})
}

func refreshNotificationStatus(ctx context.Context, tx pgx.Tx, notificationID string) error {
	var total, delivered, terminal int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status='delivered'),
		       COUNT(*) FILTER (WHERE status IN ('delivered','failed','dead_letter','cancelled'))
		FROM deliveries WHERE notification_id=$1`, notificationID).Scan(&total, &delivered, &terminal); err != nil {
		return err
	}
	status := notification.StatusDelivering
	if total > 0 && delivered == total {
		status = notification.StatusDelivered
	} else if terminal == total && delivered > 0 {
		status = notification.StatusPartiallyDelivered
	} else if terminal == total && delivered == 0 {
		status = notification.StatusFailed
	}
	_, err := tx.Exec(ctx, `UPDATE notifications SET status=$2,updated_at=now() WHERE id=$1`, notificationID, status)
	return err
}

func (store *Store) Deliveries(ctx context.Context, notificationID string) ([]delivery.Delivery, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id,notification_id,channel,destination,status,attempt_count,next_attempt_at,
		       COALESCE(last_error_code,''),COALESCE(last_error_message,''),created_at,updated_at
		FROM deliveries WHERE notification_id=$1 ORDER BY created_at,id`, notificationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]delivery.Delivery, 0)
	for rows.Next() {
		var item delivery.Delivery
		if err := rows.Scan(&item.ID, &item.NotificationID, &item.Channel, &item.Destination,
			&item.Status, &item.AttemptCount, &item.NextAttemptAt, &item.LastErrorCode,
			&item.LastErrorMessage, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) DeliveryAttempts(ctx context.Context, notificationID string) ([]delivery.Attempt, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT a.id,a.delivery_id,a.attempt_number,a.provider,a.started_at,a.finished_at,
		       a.status,COALESCE(a.response_code,0),COALESCE(a.error_code,''),
		       COALESCE(a.error_message,'')
		FROM delivery_attempts a JOIN deliveries d ON d.id=a.delivery_id
		WHERE d.notification_id=$1 ORDER BY a.started_at,a.id`, notificationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]delivery.Attempt, 0)
	for rows.Next() {
		var item delivery.Attempt
		if err := rows.Scan(&item.ID, &item.DeliveryID, &item.AttemptNumber,
			&item.Provider, &item.StartedAt, &item.FinishedAt, &item.Status,
			&item.ResponseCode, &item.ErrorCode, &item.ErrorMessage); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func decodeJSON[T any](raw []byte, target *T) error {
	return json.Unmarshal(raw, target)
}
