package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dispatch/internal/connectors"
)

func (store *Store) CreateConnectorConnection(
	ctx context.Context,
	input connectors.CreateInput,
	status, accountLabel string,
	credentialsCipher []byte,
	expiresAt *time.Time,
) (connectors.Connection, error) {
	item := connectors.Connection{
		ID: "con_" + uuid.NewString(), ConnectorID: input.ConnectorID,
		Name: input.Name, RecipientID: input.RecipientID, Status: status,
		AccountLabel: accountLabel, Config: input.Config, TokenExpiresAt: expiresAt,
		Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if item.Config == nil {
		item.Config = map[string]string{}
	}
	configJSON, err := json.Marshal(item.Config)
	if err != nil {
		return item, err
	}
	_, err = store.pool.Exec(ctx, `
		INSERT INTO connector_connections
		    (id,connector_id,name,recipient_id,status,account_label,config_json,
		     credentials_cipher,token_expires_at,last_tested_at,enabled,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,now(),true,$10,$10)`,
		item.ID, item.ConnectorID, item.Name, item.RecipientID, item.Status,
		item.AccountLabel, configJSON, credentialsCipher, item.TokenExpiresAt, item.CreatedAt)
	return item, err
}

func (store *Store) ListConnectorConnections(ctx context.Context) ([]connectors.Connection, error) {
	rows, err := store.pool.Query(ctx, connectorConnectionSelect+` ORDER BY c.created_at DESC,c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]connectors.Connection, 0)
	for rows.Next() {
		item, _, err := scanConnectorConnection(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) GetConnectorConnection(
	ctx context.Context,
	id string,
) (connectors.Connection, []byte, error) {
	item, cipher, err := scanConnectorConnection(
		store.pool.QueryRow(ctx, connectorConnectionSelect+` WHERE c.id=$1`, id),
	)
	return item, cipher, notFound(err)
}

const connectorConnectionSelect = `
	SELECT c.id,c.connector_id,c.name,c.recipient_id,c.status,
	       COALESCE(c.account_label,''),c.config_json,c.credentials_cipher,
	       c.token_expires_at,c.last_tested_at,c.last_event_at,
	       COALESCE(c.last_error,''),c.enabled,c.created_at,c.updated_at
	FROM connector_connections c`

func scanConnectorConnection(scanner interface{ Scan(...any) error }) (
	connectors.Connection,
	[]byte,
	error,
) {
	var item connectors.Connection
	var configJSON, cipher []byte
	err := scanner.Scan(
		&item.ID, &item.ConnectorID, &item.Name, &item.RecipientID, &item.Status,
		&item.AccountLabel, &configJSON, &cipher, &item.TokenExpiresAt,
		&item.LastTestedAt, &item.LastEventAt, &item.LastError, &item.Enabled,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return item, nil, err
	}
	err = json.Unmarshal(configJSON, &item.Config)
	return item, cipher, err
}

func (store *Store) UpdateConnectorState(
	ctx context.Context,
	id, status, accountLabel, lastError string,
	tested bool,
) error {
	command, err := store.pool.Exec(ctx, `
		UPDATE connector_connections
		SET status=$2,account_label=COALESCE(NULLIF($3,''),account_label),
		    last_error=NULLIF($4,''),last_tested_at=CASE WHEN $5 THEN now() ELSE last_tested_at END,
		    enabled=($2<>'disabled'),updated_at=now()
		WHERE id=$1`, id, status, accountLabel, lastError, tested)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) UpdateConnectorCredentials(
	ctx context.Context,
	id string,
	credentialsCipher []byte,
	expiresAt *time.Time,
) error {
	command, err := store.pool.Exec(ctx, `
		UPDATE connector_connections
		SET credentials_cipher=$2,token_expires_at=$3,updated_at=now()
		WHERE id=$1`, id, credentialsCipher, expiresAt)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) TouchConnectorEvent(ctx context.Context, id string) error {
	command, err := store.pool.Exec(ctx, `
		UPDATE connector_connections SET last_event_at=now(),last_error=NULL,updated_at=now()
		WHERE id=$1`, id)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) DeleteConnectorConnection(ctx context.Context, id string) error {
	command, err := store.pool.Exec(ctx, `DELETE FROM connector_connections WHERE id=$1`, id)
	if err != nil {
		return err
	}
	return requireChanged(command)
}

func (store *Store) CreateOAuthState(ctx context.Context, state connectors.OAuthState) error {
	_, err := store.pool.Exec(ctx, `
		INSERT INTO connector_oauth_states
		    (state_hash,connector_id,connection_name,recipient_id,verifier_cipher,expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		state.StateHash, state.ConnectorID, state.ConnectionName,
		state.RecipientID, state.VerifierCipher, state.ExpiresAt)
	return err
}

func (store *Store) ConsumeOAuthState(
	ctx context.Context,
	hash []byte,
) (connectors.OAuthState, error) {
	var result connectors.OAuthState
	err := pgx.BeginFunc(ctx, store.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			DELETE FROM connector_oauth_states
			WHERE state_hash=$1 AND expires_at>now()
			RETURNING state_hash,connector_id,connection_name,recipient_id,
			          verifier_cipher,expires_at`, hash).Scan(
			&result.StateHash, &result.ConnectorID, &result.ConnectionName,
			&result.RecipientID, &result.VerifierCipher, &result.ExpiresAt)
		return notFound(err)
	})
	return result, err
}

func (store *Store) RecordConnectorEvent(
	ctx context.Context,
	connectionID, externalID, notificationID, eventType string,
) error {
	_, err := store.pool.Exec(ctx, `
		INSERT INTO connector_events(connection_id,external_id,notification_id,event_type)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT(connection_id,external_id) DO NOTHING`,
		connectionID, externalID, notificationID, eventType)
	return err
}
