package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"dispatch/internal/recipient"
)

func (store *Store) CreateRecipient(ctx context.Context, item *recipient.Recipient) error {
	if item.ID == "" {
		item.ID = "rec_" + uuid.NewString()
	}
	now := time.Now().UTC()
	item.CreatedAt, item.UpdatedAt = now, now
	preferences, err := json.Marshal(item.Preferences)
	if err != nil {
		return err
	}
	_, err = store.pool.Exec(ctx, `
		INSERT INTO recipients
		    (id,name,destination_type,destination_label,email,telegram_chat_id,
		     webhook_url,preferences_json,created_at,updated_at)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),
		        NULLIF($7,''),$8,$9,$9)`,
		item.ID, item.Name, item.DestinationType, item.DestinationLabel, item.Email,
		item.TelegramChatID, item.WebhookURL, preferences, now)
	return err
}

func (store *Store) ListRecipients(ctx context.Context) ([]recipient.Recipient, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id,name,destination_type,COALESCE(destination_label,''),
		       COALESCE(email,''),COALESCE(telegram_chat_id,''),
		       COALESCE(webhook_url,''),preferences_json,created_at,updated_at
		FROM recipients ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]recipient.Recipient, 0)
	for rows.Next() {
		item, err := scanRecipient(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) GetRecipient(ctx context.Context, id string) (recipient.Recipient, error) {
	item, err := scanRecipient(store.pool.QueryRow(ctx, `
		SELECT id,name,destination_type,COALESCE(destination_label,''),
		       COALESCE(email,''),COALESCE(telegram_chat_id,''),
		       COALESCE(webhook_url,''),preferences_json,created_at,updated_at
		FROM recipients WHERE id=$1`, id))
	return item, notFound(err)
}

func scanRecipient(scanner interface{ Scan(...any) error }) (recipient.Recipient, error) {
	var item recipient.Recipient
	var preferences []byte
	err := scanner.Scan(&item.ID, &item.Name, &item.DestinationType,
		&item.DestinationLabel, &item.Email, &item.TelegramChatID,
		&item.WebhookURL, &preferences, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	err = json.Unmarshal(preferences, &item.Preferences)
	return item, err
}

func (store *Store) UpdateRecipient(ctx context.Context, item recipient.Recipient) error {
	preferences, err := json.Marshal(item.Preferences)
	if err != nil {
		return err
	}
	command, err := store.pool.Exec(ctx, `
		UPDATE recipients
		SET name=$2,destination_type=$3,destination_label=NULLIF($4,''),
		    email=NULLIF($5,''),telegram_chat_id=NULLIF($6,''),
		    webhook_url=NULLIF($7,''),preferences_json=$8,updated_at=now()
		WHERE id=$1`,
		item.ID, item.Name, item.DestinationType, item.DestinationLabel,
		item.Email, item.TelegramChatID, item.WebhookURL, preferences)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) DeleteRecipient(ctx context.Context, id string) error {
	command, err := store.pool.Exec(ctx, `DELETE FROM recipients WHERE id=$1`, id)
	if err != nil {
		return err
	}
	return requireChanged(command)
}
