CREATE TABLE IF NOT EXISTS connector_connections (
    id TEXT PRIMARY KEY,
    connector_id TEXT NOT NULL,
    name TEXT NOT NULL,
    recipient_id TEXT NOT NULL REFERENCES recipients(id),
    status TEXT NOT NULL,
    account_label TEXT,
    config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    credentials_cipher BYTEA,
    token_expires_at TIMESTAMPTZ,
    last_tested_at TIMESTAMPTZ,
    last_event_at TIMESTAMPTZ,
    last_error TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status IN ('connected','action_required','error','disabled'))
);

CREATE INDEX IF NOT EXISTS idx_connector_connections_connector
    ON connector_connections(connector_id, enabled);

CREATE INDEX IF NOT EXISTS idx_connector_connections_recipient
    ON connector_connections(recipient_id, enabled);

CREATE TABLE IF NOT EXISTS connector_oauth_states (
    state_hash BYTEA PRIMARY KEY,
    connector_id TEXT NOT NULL,
    connection_name TEXT NOT NULL,
    recipient_id TEXT NOT NULL REFERENCES recipients(id),
    verifier_cipher BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_connector_oauth_states_expiry
    ON connector_oauth_states(expires_at);

CREATE TABLE IF NOT EXISTS connector_events (
    id BIGSERIAL PRIMARY KEY,
    connection_id TEXT NOT NULL REFERENCES connector_connections(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL,
    notification_id TEXT REFERENCES notifications(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(connection_id, external_id)
);

CREATE INDEX IF NOT EXISTS idx_connector_events_received
    ON connector_events(connection_id, received_at DESC);
