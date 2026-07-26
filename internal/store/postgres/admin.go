package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"dispatch/internal/notification"
	"dispatch/internal/routing"
	"dispatch/internal/template"
)

func (store *Store) CreateTemplate(ctx context.Context, item *template.Template) error {
	if item.ID == "" {
		item.ID = "tpl_" + uuid.NewString()
	}
	now := time.Now().UTC()
	item.CreatedAt, item.UpdatedAt = now, now
	conditions, err := json.Marshal(item.Conditions)
	if err != nil {
		return err
	}
	_, err = store.pool.Exec(ctx, `
		INSERT INTO templates
		    (id,name,channel,service,conditions_json,enabled,subject_template,body_template,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$9)`,
		item.ID, item.Name, item.Channel, item.Service, conditions, item.Enabled,
		item.SubjectTemplate, item.BodyTemplate, now)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (store *Store) ListTemplates(ctx context.Context) ([]template.Template, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id,name,channel,service,conditions_json,enabled,
		       COALESCE(subject_template,''),body_template,created_at,updated_at
		FROM templates
		ORDER BY service, jsonb_array_length(conditions_json) DESC, updated_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]template.Template, 0)
	for rows.Next() {
		var item template.Template
		var conditions []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.Channel, &item.Service,
			&conditions, &item.Enabled, &item.SubjectTemplate, &item.BodyTemplate,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(conditions, &item.Conditions); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) GetTemplate(ctx context.Context, id string) (template.Template, error) {
	var item template.Template
	var conditions []byte
	err := store.pool.QueryRow(ctx, `
		SELECT id,name,channel,service,conditions_json,enabled,
		       COALESCE(subject_template,''),body_template,created_at,updated_at
		FROM templates WHERE id=$1`, id).Scan(
		&item.ID, &item.Name, &item.Channel, &item.Service, &conditions, &item.Enabled,
		&item.SubjectTemplate, &item.BodyTemplate, &item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(conditions, &item.Conditions)
	}
	return item, notFound(err)
}

func (store *Store) UpdateTemplate(ctx context.Context, item template.Template) error {
	conditions, err := json.Marshal(item.Conditions)
	if err != nil {
		return err
	}
	command, err := store.pool.Exec(ctx, `
		UPDATE templates
		SET name=$2,channel=$3,service=$4,conditions_json=$5,enabled=$6,
		    subject_template=NULLIF($7,''),body_template=$8,updated_at=now()
		WHERE id=$1`,
		item.ID, item.Name, item.Channel, item.Service, conditions, item.Enabled,
		item.SubjectTemplate, item.BodyTemplate)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) DeleteTemplate(ctx context.Context, id string) error {
	command, err := store.pool.Exec(ctx, `DELETE FROM templates WHERE id=$1`, id)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) MatchTemplate(
	ctx context.Context,
	item notification.Notification,
	channel string,
) (*template.Template, error) {
	items, err := store.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	return template.Select(items, item, channel), nil
}

func (store *Store) CreateRule(ctx context.Context, item *routing.Rule) error {
	if item.ID == "" {
		item.ID = "rul_" + uuid.NewString()
	}
	now := time.Now().UTC()
	item.CreatedAt, item.UpdatedAt = now, now
	condition, err := json.Marshal(item.Condition)
	if err != nil {
		return err
	}
	action, err := json.Marshal(item.Action)
	if err != nil {
		return err
	}
	_, err = store.pool.Exec(ctx, `
		INSERT INTO routing_rules(id,name,priority,condition_json,action_json,enabled,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$7)`,
		item.ID, item.Name, item.Priority, condition, action, item.Enabled, now)
	return err
}

func (store *Store) ListRules(ctx context.Context) ([]routing.Rule, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id,name,priority,condition_json,action_json,enabled,created_at,updated_at
		FROM routing_rules ORDER BY priority,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]routing.Rule, 0)
	for rows.Next() {
		var item routing.Rule
		var condition, action []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.Priority, &condition,
			&action, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(condition, &item.Condition); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(action, &item.Action); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) UpdateRule(ctx context.Context, item routing.Rule) error {
	condition, err := json.Marshal(item.Condition)
	if err != nil {
		return err
	}
	action, err := json.Marshal(item.Action)
	if err != nil {
		return err
	}
	command, err := store.pool.Exec(ctx, `
		UPDATE routing_rules SET name=$2,priority=$3,condition_json=$4,action_json=$5,
		    enabled=$6,updated_at=now() WHERE id=$1`,
		item.ID, item.Name, item.Priority, condition, action, item.Enabled)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) DeleteRule(ctx context.Context, id string) error {
	command, err := store.pool.Exec(ctx, `DELETE FROM routing_rules WHERE id=$1`, id)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

type Dashboard struct {
	Notifications24H int64            `json:"notifications_24h"`
	Delivered        int64            `json:"delivered"`
	Failed           int64            `json:"failed"`
	WaitingRetry     int64            `json:"waiting_retry"`
	DeadLetter       int64            `json:"dead_letter"`
	AIFallbackRate   float64          `json:"ai_fallback_rate"`
	ByChannel        map[string]int64 `json:"deliveries_by_channel"`
	RecentFailures   []FailureSummary `json:"recent_failures"`
}

type FailureSummary struct {
	NotificationID string    `json:"notification_id"`
	Subject        string    `json:"subject"`
	Channel        string    `json:"channel"`
	Error          string    `json:"error"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (store *Store) Dashboard(ctx context.Context) (Dashboard, error) {
	result := Dashboard{ByChannel: map[string]int64{}, RecentFailures: []FailureSummary{}}
	err := store.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE created_at >= now()-interval '24 hours'),
		       COUNT(*) FILTER (WHERE status='delivered'),
		       COUNT(*) FILTER (WHERE status='failed')
		FROM notifications`).Scan(&result.Notifications24H, &result.Delivered, &result.Failed)
	if err != nil {
		return result, err
	}
	if err := store.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status='retry_wait'),
		       COUNT(*) FILTER (WHERE status='dead_letter')
		FROM deliveries`).Scan(&result.WaitingRetry, &result.DeadLetter); err != nil {
		return result, err
	}
	var totalAI, fallbackAI int64
	if err := store.pool.QueryRow(ctx, `
		SELECT COUNT(*),COUNT(*) FILTER (WHERE status='fallback_used') FROM ai_decisions`).
		Scan(&totalAI, &fallbackAI); err != nil {
		return result, err
	}
	if totalAI > 0 {
		result.AIFallbackRate = float64(fallbackAI) / float64(totalAI)
	}
	rows, err := store.pool.Query(ctx, `
		SELECT channel,COUNT(*) FROM deliveries GROUP BY channel ORDER BY channel`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var channel string
		var count int64
		if err := rows.Scan(&channel, &count); err != nil {
			rows.Close()
			return result, err
		}
		result.ByChannel[channel] = count
	}
	rows.Close()
	rows, err = store.pool.Query(ctx, `
		SELECT n.id,n.subject,d.channel,COALESCE(d.last_error_message,''),d.updated_at
		FROM deliveries d JOIN notifications n ON n.id=d.notification_id
		WHERE d.status IN ('failed','dead_letter','retry_wait')
		ORDER BY d.updated_at DESC LIMIT 10`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item FailureSummary
		if err := rows.Scan(&item.NotificationID, &item.Subject, &item.Channel, &item.Error, &item.UpdatedAt); err != nil {
			return result, err
		}
		result.RecentFailures = append(result.RecentFailures, item)
	}
	return result, rows.Err()
}

func (store *Store) SeedDemo(ctx context.Context) error {
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM recipients`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := store.pool.Exec(ctx, `
		INSERT INTO recipients
		    (id,name,email,telegram_chat_id,webhook_url,preferences_json)
		VALUES
		    ('rec_demo_ops','Operations','ops@example.test','-100123456','',
		     '{"default_channels":["email"],"time_zone":"UTC"}'),
		    ('rec_demo_finance','Finance','finance@example.test','','',
		     '{"default_channels":["email"],"time_zone":"UTC"}');

		INSERT INTO routing_rules(id,name,priority,condition_json,action_json,enabled)
		VALUES ('rul_demo_payment','Critical payments',10,
		        '{"event_type":"payment.failed"}','{"channels":["email","telegram"]}',true);

		INSERT INTO templates(id,name,channel,subject_template,body_template)
		VALUES ('tpl_demo_email','Default email','email','{{subject}}','{{body}}');
	`)
	return err
}
