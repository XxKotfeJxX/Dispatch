ALTER TABLE recipients
    ADD COLUMN IF NOT EXISTS destination_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS destination_label TEXT;

UPDATE recipients
SET destination_type = CASE
    WHEN preferences_json->'default_channels' ? 'telegram' THEN 'telegram'
    WHEN preferences_json->'default_channels' ? 'webhook' THEN 'webhook'
    WHEN preferences_json->'default_channels' ? 'email' THEN 'email'
    WHEN telegram_chat_id IS NOT NULL AND telegram_chat_id <> '' THEN 'telegram'
    WHEN webhook_url IS NOT NULL AND webhook_url <> '' THEN 'webhook'
    ELSE 'email'
END
WHERE destination_type = '';

CREATE TABLE IF NOT EXISTS recipient_setups (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    target TEXT,
    code_hash BYTEA NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    recipient_id TEXT REFERENCES recipients(id),
    attempts INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_recipient_setups_pending
    ON recipient_setups(kind, status, expires_at);
