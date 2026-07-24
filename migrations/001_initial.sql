CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS recipients (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT,
    telegram_chat_id TEXT,
    webhook_url TEXT,
    preferences_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    recipient_id TEXT NOT NULL REFERENCES recipients(id),
    event_type TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    requested_channels TEXT[] NOT NULL DEFAULT '{}',
    scheduled_at TIMESTAMPTZ,
    category TEXT,
    priority TEXT NOT NULL DEFAULT 'normal',
    summary TEXT,
    status TEXT NOT NULL DEFAULT 'received',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_event_type ON notifications(event_type, created_at DESC);

CREATE TABLE IF NOT EXISTS routing_rules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    priority INTEGER NOT NULL,
    condition_json JSONB NOT NULL,
    action_json JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_routing_rules_priority ON routing_rules(enabled, priority, id);

CREATE TABLE IF NOT EXISTS ai_decisions (
    id TEXT PRIMARY KEY,
    notification_id TEXT REFERENCES notifications(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    category TEXT,
    priority TEXT,
    summary TEXT,
    recommended_channels TEXT[] NOT NULL DEFAULT '{}',
    send_immediately BOOLEAN NOT NULL DEFAULT false,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason_codes TEXT[] NOT NULL DEFAULT '{}',
    raw_response_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    fallback_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_decisions_notification ON ai_decisions(notification_id, created_at DESC);

CREATE TABLE IF NOT EXISTS deliveries (
    id TEXT PRIMARY KEY,
    notification_id TEXT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    destination TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error_code TEXT,
    last_error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(notification_id, channel, destination)
);

CREATE INDEX IF NOT EXISTS idx_deliveries_retry ON deliveries(status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_deliveries_notification ON deliveries(notification_id, created_at);

CREATE TABLE IF NOT EXISTS delivery_attempts (
    id TEXT PRIMARY KEY,
    delivery_id TEXT NOT NULL REFERENCES deliveries(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL,
    provider TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    status TEXT NOT NULL,
    response_code INTEGER,
    error_code TEXT,
    error_message TEXT,
    UNIQUE(delivery_id, attempt_number)
);

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_jobs_claim ON jobs(status, available_at, created_at);
CREATE INDEX IF NOT EXISTS idx_jobs_locked ON jobs(status, locked_at);

CREATE TABLE IF NOT EXISTS templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    channel TEXT NOT NULL,
    subject_template TEXT,
    body_template TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    details_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_events(entity_type, entity_id, created_at);
