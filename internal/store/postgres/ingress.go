package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"dispatch/internal/ingress"
)

func (store *Store) CreateIngressSource(
	ctx context.Context,
	input ingress.CreateInput,
	secretHash, secretCipher []byte,
) (ingress.Source, error) {
	item := ingress.Source{
		ID: "src_" + uuid.NewString(), Name: input.Name, Slug: input.Slug,
		Provider: input.Provider, RecipientID: input.RecipientID, AuthMode: input.AuthMode,
		AuthHeader: input.AuthHeader, Mapping: input.Mapping, Enabled: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	mapping, err := json.Marshal(item.Mapping)
	if err != nil {
		return item, err
	}
	_, err = store.pool.Exec(ctx, `
		INSERT INTO ingress_sources
		    (id,name,slug,provider,recipient_id,auth_mode,auth_header,signature_header,
		     secret_hash,secret_cipher,mapping_json,enabled,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10,$11,true,$12,$12)`,
		item.ID, item.Name, item.Slug, item.Provider, item.RecipientID, item.AuthMode,
		item.AuthHeader, item.SignatureHeader, secretHash, secretCipher, mapping, item.CreatedAt)
	if isUniqueViolation(err) {
		return item, ErrConflict
	}
	return item, err
}

func (store *Store) ListIngressSources(ctx context.Context) ([]ingress.Source, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id,name,slug,provider,recipient_id,auth_mode,COALESCE(auth_header,''),
		       COALESCE(signature_header,''),mapping_json,enabled,created_at,updated_at
		FROM ingress_sources ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ingress.Source, 0)
	for rows.Next() {
		item, err := scanIngressSource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) GetIngressSourceBySlug(
	ctx context.Context,
	slug string,
) (ingress.Source, []byte, []byte, error) {
	var hash, cipher []byte
	var mapping []byte
	var item ingress.Source
	err := store.pool.QueryRow(ctx, `
		SELECT id,name,slug,provider,recipient_id,auth_mode,COALESCE(auth_header,''),
		       COALESCE(signature_header,''),mapping_json,enabled,created_at,updated_at,
		       secret_hash,secret_cipher
		FROM ingress_sources WHERE slug=$1`, slug).Scan(
		&item.ID, &item.Name, &item.Slug, &item.Provider, &item.RecipientID,
		&item.AuthMode, &item.AuthHeader, &item.SignatureHeader, &mapping,
		&item.Enabled, &item.CreatedAt, &item.UpdatedAt, &hash, &cipher)
	if err != nil {
		return item, nil, nil, notFound(err)
	}
	err = json.Unmarshal(mapping, &item.Mapping)
	return item, hash, cipher, err
}

func scanIngressSource(scanner interface{ Scan(...any) error }) (ingress.Source, error) {
	var item ingress.Source
	var mapping []byte
	err := scanner.Scan(&item.ID, &item.Name, &item.Slug, &item.Provider,
		&item.RecipientID, &item.AuthMode, &item.AuthHeader, &item.SignatureHeader,
		&mapping, &item.Enabled, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	err = json.Unmarshal(mapping, &item.Mapping)
	return item, err
}

func (store *Store) RotateIngressSecret(
	ctx context.Context,
	id string,
	secretHash, secretCipher []byte,
) error {
	command, err := store.pool.Exec(ctx, `
		UPDATE ingress_sources SET secret_hash=$2,secret_cipher=$3,updated_at=now()
		WHERE id=$1`, id, secretHash, secretCipher)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) SetIngressSourceEnabled(ctx context.Context, id string, enabled bool) error {
	command, err := store.pool.Exec(ctx, `
		UPDATE ingress_sources SET enabled=$2,updated_at=now() WHERE id=$1`, id, enabled)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) DeleteIngressSource(ctx context.Context, id string) error {
	command, err := store.pool.Exec(ctx, `DELETE FROM ingress_sources WHERE id=$1`, id)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) RecordIngressEvent(ctx context.Context, event ingress.Event) error {
	_, err := store.pool.Exec(ctx, `
		INSERT INTO ingress_events
		    (source_id,external_id,notification_id,event_type,payload_digest,received_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT(source_id,external_id) DO NOTHING`,
		event.SourceID, event.ExternalID, event.NotificationID,
		event.EventType, event.PayloadDigest, event.ReceivedAt)
	return err
}

func (store *Store) IngressEvents(ctx context.Context, sourceID string, limit int) ([]ingress.Event, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := store.pool.Query(ctx, `
		SELECT id,source_id,external_id,notification_id,event_type,payload_digest,received_at
		FROM ingress_events WHERE source_id=$1 ORDER BY received_at DESC LIMIT $2`,
		sourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ingress.Event, 0)
	for rows.Next() {
		var item ingress.Event
		if err := rows.Scan(&item.ID, &item.SourceID, &item.ExternalID,
			&item.NotificationID, &item.EventType, &item.PayloadDigest,
			&item.ReceivedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
