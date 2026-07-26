# Universal ingress

Dispatch can turn practically any authenticated JSON webhook into a normal notification. Each source has its own endpoint, recipient, routing channels, authentication secret, field mapping, event history, and deduplication boundary.

## Create a source

1. Open **Sources** in the console.
2. Choose a provider preset or **Generic JSON**.
3. Select the Dispatch recipient and optional explicit delivery channels.
4. Adjust the dot-path mappings if the producer's JSON differs from the preset.
5. Create the source and immediately save the generated secret. It is shown once.
6. Configure the producer to send JSON to `https://dispatch.example/ingest/v1/<slug>`.

Cloud services cannot call `localhost` or a private LAN address. Expose Dispatch through a trusted HTTPS reverse proxy or a temporary development tunnel, and keep the ingress endpoint authenticated.

## Authentication

| Mode | Producer sends | Typical preset |
|---|---|---|
| `bearer` | `Authorization: Bearer <secret>` or `X-Dispatch-Ingest-Key` | Generic, Discord, Sentry, Grafana |
| `header` | Generated secret in the configured header | GitLab |
| `hmac_sha256` | `sha256=<hex HMAC of raw body>` | GitHub |
| `slack_signature` | Slack timestamp and `v0` signature | Slack |
| `stripe_signature` | Stripe timestamp and `v1` signature | Stripe |

Bearer and custom-header secrets are stored as one-way hashes. Signing secrets are encrypted with AES-GCM because Dispatch must use them to verify signatures. Set `INGRESS_ENCRYPTION_KEY` to a long, stable, independent value and back it up with the database. Changing it makes existing HMAC, Slack, and Stripe sources unverifiable; rotate those source secrets afterward if the key is lost.

Slack and Stripe signatures are rejected outside a five-minute replay window. GitHub, Slack, and Stripe signatures are calculated over the exact raw request body.

## Generic JSON example

With the default mapping, send:

```bash
curl -X POST https://dispatch.example/ingest/v1/my-system \
  -H "Authorization: Bearer <one-time-source-secret>" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "evt-42",
    "event_type": "build.failed",
    "subject": "Production build failed",
    "body": "The deploy job exited with status 1."
  }'
```

Nested properties use dot paths such as `payload.alert.title`; array indexes are supported, for example `events.0.message`. Missing fields fall back to the source defaults. When an explicit event ID is present, repeated delivery of that source/event pair returns the original notification instead of sending it again. Without an ID, Dispatch deduplicates identical raw payloads.

## Provider notes

- **GitHub:** create a repository or organization webhook, use the generated secret, choose `application/json`, and point it to the source endpoint. Dispatch verifies `X-Hub-Signature-256` and uses `X-GitHub-Delivery` for deduplication.
- **GitLab:** use the source secret as the webhook **Secret token**. Dispatch verifies `X-Gitlab-Token`.
- **Slack:** copy the app signing secret from **Basic Information** into the source creation form, then set the source endpoint as the Event Subscriptions request URL. The URL verification challenge is handled automatically.
- **Stripe:** choose the future source URL from its slug, add that URL in Workbench/Webhooks, reveal the endpoint signing secret, then paste it into the Dispatch source creation form. Dispatch verifies `Stripe-Signature`.
- **Sentry and Grafana:** configure a bearer header directly where supported, or place a small authenticated relay/reverse proxy in front if that product cannot set one.

## Discord is not a webhook source

Discord messages arrive over the [Gateway WebSocket](https://docs.discord.com/developers/events/gateway), so the consumer Discord integration is managed separately from universal ingress. Do not create a Discord source.

The deployment owner configures `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET`, and `DISCORD_BOT_TOKEN`. The user then connects from the Discord card, chooses a recipient and mode, and selects a server in Discord. The normal Compose stack runs the bridge automatically. Connection ownership, the installed guild, event deduplication, and routing live in the connector tables.

Direct messages to the bot are an exception to the broad [Message Content privileged intent](https://docs.discord.com/developers/events/gateway#message-content-intent). To read ordinary server message text, enable Message Content Intent in the Developer Portal and set `DISCORD_MESSAGE_CONTENT_INTENT=true`.

## Rotation and diagnostics

Use **Rotate secret** on a source, update the producer immediately, and then test it. Rotation invalidates the previous secret. Disable a source to stop traffic without deleting its configuration. The source event list links accepted external IDs to Dispatch notification IDs; notification details contain the subsequent routing and delivery audit trail.
