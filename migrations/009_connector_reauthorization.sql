ALTER TABLE connector_oauth_states
    ADD COLUMN IF NOT EXISTS connection_id TEXT
    REFERENCES connector_connections(id) ON DELETE CASCADE;
