UPDATE recipients
SET destination_type = CASE
    WHEN preferences_json->'default_channels' ? 'telegram' THEN 'telegram'
    WHEN preferences_json->'default_channels' ? 'webhook' THEN 'webhook'
    WHEN preferences_json->'default_channels' ? 'email' THEN 'email'
    WHEN telegram_chat_id IS NOT NULL AND telegram_chat_id <> '' THEN 'telegram'
    WHEN webhook_url IS NOT NULL AND webhook_url <> '' THEN 'webhook'
    ELSE 'email'
END
WHERE destination_label IS NULL;
