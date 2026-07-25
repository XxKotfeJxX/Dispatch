CREATE TABLE IF NOT EXISTS ingress_sources (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    provider TEXT NOT NULL,
    recipient_id TEXT NOT NULL REFERENCES recipients(id),
    auth_mode TEXT NOT NULL,
    auth_header TEXT,
    signature_header TEXT,
    secret_hash BYTEA,
    secret_cipher BYTEA,
    mapping_json JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (auth_mode IN ('bearer','header','hmac_sha256','slack_signature','stripe_signature')),
    CHECK (secret_hash IS NOT NULL OR secret_cipher IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_ingress_sources_recipient
    ON ingress_sources(recipient_id, enabled);

CREATE TABLE IF NOT EXISTS ingress_events (
    id BIGSERIAL PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES ingress_sources(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    payload_digest TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(source_id, external_id)
);

CREATE INDEX IF NOT EXISTS idx_ingress_events_received
    ON ingress_events(source_id, received_at DESC);
