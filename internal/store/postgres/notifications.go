package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dispatch/internal/ai"
	"dispatch/internal/notification"
)

func (store *Store) CreateNotification(ctx context.Context, item *notification.Notification) (bool, error) {
	if item.ID == "" {
		item.ID = "not_" + uuid.NewString()
	}
	now := time.Now().UTC()
	item.CreatedAt, item.UpdatedAt, item.Status = now, now, notification.StatusReceived
	if item.Priority == "" {
		item.Priority = "normal"
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	if item.RequestedChannels == nil {
		item.RequestedChannels = []string{}
	}
	metadata, err := json.Marshal(item.Metadata)
	if err != nil {
		return false, err
	}
	created := false
	err = pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `
			INSERT INTO notifications
			    (id,idempotency_key,recipient_id,event_type,subject,body,metadata_json,
			     requested_channels,scheduled_at,priority,status,created_at,updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
			ON CONFLICT (idempotency_key) DO NOTHING`,
			item.ID, item.IdempotencyKey, item.RecipientID, item.EventType, item.Subject,
			item.Body, metadata, item.RequestedChannels, item.ScheduledAt, item.Priority,
			item.Status, now)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return tx.QueryRow(ctx, `
				SELECT id,recipient_id,event_type,subject,body,metadata_json,requested_channels,
				       scheduled_at,COALESCE(category,''),priority,COALESCE(summary,''),status,
				       created_at,updated_at
				FROM notifications WHERE idempotency_key=$1`, item.IdempotencyKey).Scan(
				&item.ID, &item.RecipientID, &item.EventType, &item.Subject, &item.Body,
				&metadata, &item.RequestedChannels, &item.ScheduledAt, &item.Category,
				&item.Priority, &item.Summary, &item.Status, &item.CreatedAt, &item.UpdatedAt)
		}
		created = true
		jobID := "job_" + uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO jobs(id,type,payload_json,status,max_attempts,available_at)
			VALUES ($1,'process_notification',jsonb_build_object('notification_id',$2::text),'queued',5,now())`,
			jobID, item.ID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO audit_events(entity_type,entity_id,event_type,details_json)
			VALUES ('notification',$1,'notification.received',jsonb_build_object('job_id',$2::text))`,
			item.ID, jobID)
		return err
	})
	if err != nil {
		return false, err
	}
	if !created {
		_ = json.Unmarshal(metadata, &item.Metadata)
	}
	return created, nil
}

func (store *Store) GetNotification(ctx context.Context, id string) (notification.Notification, error) {
	item, err := scanNotification(store.pool.QueryRow(ctx, notificationSelect+` WHERE n.id=$1`, id))
	return item, notFound(err)
}

const notificationSelect = `
	SELECT n.id,n.idempotency_key,n.recipient_id,n.event_type,n.subject,n.body,
	       n.metadata_json,n.requested_channels,n.scheduled_at,COALESCE(n.category,''),
	       n.priority,COALESCE(n.summary,''),n.status,n.created_at,n.updated_at
	FROM notifications n`

func scanNotification(scanner interface{ Scan(...any) error }) (notification.Notification, error) {
	var item notification.Notification
	var metadata []byte
	err := scanner.Scan(&item.ID, &item.IdempotencyKey, &item.RecipientID, &item.EventType,
		&item.Subject, &item.Body, &metadata, &item.RequestedChannels, &item.ScheduledAt,
		&item.Category, &item.Priority, &item.Summary, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	err = json.Unmarshal(metadata, &item.Metadata)
	return item, err
}

func (store *Store) ListNotifications(ctx context.Context, filter notification.Filter) ([]notification.Notification, int64, error) {
	where := []string{"TRUE"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if filter.Status != "" {
		add("n.status=$%d", filter.Status)
	}
	if filter.Priority != "" {
		add("n.priority=$%d", filter.Priority)
	}
	if filter.EventType != "" {
		add("n.event_type=$%d", filter.EventType)
	}
	if filter.Channel != "" {
		add("EXISTS(SELECT 1 FROM deliveries d WHERE d.notification_id=n.id AND d.channel=$%d)", filter.Channel)
	}
	if filter.AIFallback != nil {
		if *filter.AIFallback {
			where = append(where, `EXISTS(SELECT 1 FROM ai_decisions a WHERE a.notification_id=n.id AND a.status='fallback_used')`)
		} else {
			where = append(where, `NOT EXISTS(SELECT 1 FROM ai_decisions a WHERE a.notification_id=n.id AND a.status='fallback_used')`)
		}
	}
	if filter.From != nil {
		add("n.created_at >= $%d", *filter.From)
	}
	if filter.To != nil {
		add("n.created_at <= $%d", *filter.To)
	}
	queryWhere := strings.Join(where, " AND ")
	var total int64
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications n WHERE `+queryWhere, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit < 1 || limit > 200 {
		limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	args = append(args, limit, filter.Offset)
	rows, err := store.pool.Query(ctx, notificationSelect+` WHERE `+queryWhere+
		fmt.Sprintf(` ORDER BY n.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]notification.Notification, 0, limit)
	for rows.Next() {
		item, err := scanNotification(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, item)
	}
	return result, total, rows.Err()
}

func (store *Store) TransitionNotification(ctx context.Context, id string, to notification.Status) error {
	return pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		var from notification.Status
		if err := tx.QueryRow(ctx, `SELECT status FROM notifications WHERE id=$1 FOR UPDATE`, id).Scan(&from); err != nil {
			return notFound(err)
		}
		if from == to {
			return nil
		}
		if !notification.CanTransition(from, to) {
			return fmt.Errorf("%w: %s to %s", ErrInvalidState, from, to)
		}
		if _, err := tx.Exec(ctx, `UPDATE notifications SET status=$2,updated_at=now() WHERE id=$1`, id, to); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_events(entity_type,entity_id,event_type,details_json)
			VALUES ('notification',$1,$2,jsonb_build_object('from',$3::text,'to',$4::text))`,
			id, "notification."+string(to), from, to)
		return err
	})
}

func (store *Store) SaveAIDecision(ctx context.Context, record ai.Record) error {
	raw, err := json.Marshal(record.RawResponse)
	if err != nil {
		return err
	}
	_, err = store.pool.Exec(ctx, `
		INSERT INTO ai_decisions
		    (id,notification_id,provider,model,prompt_version,category,priority,summary,
		     recommended_channels,send_immediately,confidence,reason_codes,raw_response_json,
		     duration_ms,status,fallback_reason,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,NULLIF($16,''),$17)`,
		record.ID, record.NotificationID, record.Provider, record.Model, record.PromptVersion,
		record.Decision.Category, record.Decision.Priority, record.Decision.Summary,
		record.Decision.RecommendedChannels, record.Decision.SendImmediately,
		record.Decision.Confidence, record.Decision.ReasonCodes, raw, record.DurationMS,
		record.Status, record.FallbackReason, record.CreatedAt)
	return err
}

func (store *Store) AIDecisions(ctx context.Context, notificationID string) ([]ai.Record, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id,notification_id,provider,model,prompt_version,COALESCE(category,''),
		       COALESCE(priority,''),COALESCE(summary,''),recommended_channels,
		       send_immediately,confidence,reason_codes,raw_response_json,duration_ms,status,
		       COALESCE(fallback_reason,''),created_at
		FROM ai_decisions WHERE notification_id=$1 ORDER BY created_at DESC`, notificationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ai.Record, 0)
	for rows.Next() {
		var record ai.Record
		var raw []byte
		if err := rows.Scan(&record.ID, &record.NotificationID, &record.Provider, &record.Model,
			&record.PromptVersion, &record.Decision.Category, &record.Decision.Priority,
			&record.Decision.Summary, &record.Decision.RecommendedChannels,
			&record.Decision.SendImmediately, &record.Decision.Confidence,
			&record.Decision.ReasonCodes, &raw, &record.DurationMS, &record.Status,
			&record.FallbackReason, &record.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &record.RawResponse); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (store *Store) SetNotificationRouting(ctx context.Context, id, category, priority, summary string) error {
	command, err := store.pool.Exec(ctx, `
		UPDATE notifications SET category=$2,priority=$3,summary=$4,status='queued',updated_at=now()
		WHERE id=$1 AND status IN ('received','analyzing','failed')`, id, category, priority, summary)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

func (store *Store) CancelNotification(ctx context.Context, id string) error {
	return pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		command, err := tx.Exec(ctx, `
			UPDATE notifications SET status='cancelled',updated_at=now()
			WHERE id=$1 AND status NOT IN ('delivered','cancelled')`, id)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return ErrInvalidState
		}
		if _, err := tx.Exec(ctx, `
			UPDATE deliveries SET status='cancelled',updated_at=now()
			WHERE notification_id=$1 AND status IN ('queued','retry_wait')`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE jobs SET status='cancelled',updated_at=now()
			WHERE status='queued' AND (payload_json->>'notification_id'=$1 OR payload_json->>'delivery_id' IN
			    (SELECT id FROM deliveries WHERE notification_id=$1))`, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO audit_events(entity_type,entity_id,event_type,details_json)
			VALUES ('notification',$1,'notification.cancelled','{}')`, id)
		return err
	})
}

func (store *Store) RetryNotification(ctx context.Context, id string) error {
	return pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		var status notification.Status
		if err := tx.QueryRow(ctx, `SELECT status FROM notifications WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
			return notFound(err)
		}
		if status != notification.StatusFailed && status != notification.StatusPartiallyDelivered {
			return ErrInvalidState
		}
		if _, err := tx.Exec(ctx, `UPDATE notifications SET status='queued',updated_at=now() WHERE id=$1`, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id FROM deliveries WHERE notification_id=$1 AND status IN ('failed','dead_letter')`, id)
		if err != nil {
			return err
		}
		var deliveryIDs []string
		for rows.Next() {
			var deliveryID string
			if err := rows.Scan(&deliveryID); err != nil {
				rows.Close()
				return err
			}
			deliveryIDs = append(deliveryIDs, deliveryID)
		}
		rows.Close()
		for _, deliveryID := range deliveryIDs {
			if _, err := tx.Exec(ctx, `
				UPDATE deliveries SET status='queued',attempt_count=0,next_attempt_at=now(),
				    last_error_code=NULL,last_error_message=NULL,updated_at=now()
				WHERE id=$1`, deliveryID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO jobs(id,type,payload_json,status,max_attempts,available_at)
				VALUES ($1,'deliver',jsonb_build_object('delivery_id',$2::text,'notification_id',$3::text),'queued',5,now())`,
				"job_"+uuid.NewString(), deliveryID, id); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO audit_events(entity_type,entity_id,event_type,details_json)
			VALUES ('notification',$1,'notification.manual_retry','{}')`, id)
		return err
	})
}

func isUniqueViolation(err error) bool {
	var target interface{ SQLState() string }
	return errors.As(err, &target) && target.SQLState() == "23505"
}
