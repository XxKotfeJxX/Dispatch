package postgres

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dispatch/internal/recipient"
)

func (store *Store) CreateRecipientSetup(ctx context.Context, item *recipient.Setup) error {
	if item.ID == "" {
		item.ID = "rst_" + uuid.NewString()
	}
	now := time.Now().UTC()
	item.Status, item.CreatedAt, item.UpdatedAt = "pending", now, now
	_, err := store.pool.Exec(ctx, `
		INSERT INTO recipient_setups
		    (id,kind,name,target,code_hash,status,expires_at,created_at,updated_at)
		VALUES ($1,$2,$3,NULLIF($4,''),$5,'pending',$6,$7,$7)`,
		item.ID, item.Kind, item.Name, item.Target, item.CodeHash, item.ExpiresAt, now)
	return err
}

func (store *Store) GetRecipientSetup(ctx context.Context, id string) (recipient.Setup, error) {
	var item recipient.Setup
	err := store.pool.QueryRow(ctx, `
		SELECT id,kind,name,COALESCE(target,''),status,COALESCE(recipient_id,''),
		       attempts,expires_at,created_at,updated_at
		FROM recipient_setups WHERE id=$1`, id).Scan(
		&item.ID, &item.Kind, &item.Name, &item.Target, &item.Status,
		&item.RecipientID, &item.Attempts, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt)
	return item, notFound(err)
}

func (store *Store) CompleteMailpitRecipientSetup(
	ctx context.Context,
	id string,
	codeHash []byte,
) (recipient.Recipient, error) {
	var result recipient.Recipient
	err := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		var setup recipient.Setup
		err := tx.QueryRow(ctx, `
			UPDATE recipient_setups
			SET attempts=attempts+1,updated_at=now()
			WHERE id=$1
			RETURNING id,kind,name,COALESCE(target,''),status,code_hash,
			          attempts,expires_at,created_at,updated_at`,
			id).Scan(&setup.ID, &setup.Kind, &setup.Name, &setup.Target,
			&setup.Status, &setup.CodeHash, &setup.Attempts, &setup.ExpiresAt,
			&setup.CreatedAt, &setup.UpdatedAt)
		if err != nil {
			return notFound(err)
		}
		if setup.Kind != "mailpit" || setup.Status != "pending" ||
			time.Now().UTC().After(setup.ExpiresAt) || setup.Attempts > 5 ||
			!equalBytes(setup.CodeHash, codeHash) {
			return ErrInvalidState
		}
		result = recipient.Recipient{
			ID: "rec_" + uuid.NewString(), Name: setup.Name,
			DestinationType: "mailpit", DestinationLabel: setup.Target,
			Email: setup.Target,
			Preferences: recipient.Preferences{
				DefaultChannels: []string{"email"}, DisabledChannels: []string{},
				TimeZone: "UTC",
			},
		}
		if err := insertPairedRecipient(ctx, tx, &result); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE recipient_setups
			SET status='completed',recipient_id=$2,updated_at=now()
			WHERE id=$1`, setup.ID, result.ID)
		return err
	})
	return result, err
}

func (store *Store) CompleteTelegramRecipientSetup(
	ctx context.Context,
	codeHash []byte,
	chatID, label string,
) (recipient.Recipient, error) {
	var result recipient.Recipient
	err := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		var setup recipient.Setup
		err := tx.QueryRow(ctx, `
			SELECT id,kind,name,COALESCE(target,''),status,expires_at
			FROM recipient_setups
			WHERE code_hash=$1 AND kind='telegram' AND status='pending'
			FOR UPDATE`, codeHash).Scan(
			&setup.ID, &setup.Kind, &setup.Name, &setup.Target,
			&setup.Status, &setup.ExpiresAt)
		if err != nil {
			return notFound(err)
		}
		if time.Now().UTC().After(setup.ExpiresAt) {
			return ErrInvalidState
		}
		result = recipient.Recipient{
			ID: "rec_" + uuid.NewString(), Name: setup.Name,
			DestinationType: "telegram", DestinationLabel: label,
			TelegramChatID: chatID,
			Preferences: recipient.Preferences{
				DefaultChannels: []string{"telegram"}, DisabledChannels: []string{},
				TimeZone: "UTC",
			},
		}
		if err := insertPairedRecipient(ctx, tx, &result); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE recipient_setups
			SET status='completed',recipient_id=$2,target=$3,updated_at=now()
			WHERE id=$1`, setup.ID, result.ID, label)
		return err
	})
	return result, err
}

func insertPairedRecipient(ctx context.Context, tx pgx.Tx, item *recipient.Recipient) error {
	now := time.Now().UTC()
	item.CreatedAt, item.UpdatedAt = now, now
	preferences, err := jsonBytes(item.Preferences)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO recipients
		    (id,name,destination_type,destination_label,email,telegram_chat_id,
		     webhook_url,preferences_json,created_at,updated_at)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),
		        NULLIF($7,''),$8,$9,$9)`,
		item.ID, item.Name, item.DestinationType, item.DestinationLabel,
		item.Email, item.TelegramChatID, item.WebhookURL, preferences, now)
	return err
}

func jsonBytes(value any) ([]byte, error) {
	return json.Marshal(value)
}

func equalBytes(left, right []byte) bool {
	return subtle.ConstantTimeCompare(left, right) == 1
}
