ALTER TABLE templates
    ADD COLUMN IF NOT EXISTS service TEXT NOT NULL DEFAULT 'any',
    ADD COLUMN IF NOT EXISTS conditions_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT true;

UPDATE templates
SET channel = 'all'
WHERE id = 'tpl_demo_email' AND channel = 'email';

CREATE INDEX IF NOT EXISTS idx_templates_match
    ON templates(enabled, service, channel, updated_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_templates_one_fallback
    ON templates(service, channel)
    WHERE enabled AND jsonb_array_length(conditions_json) = 0;
