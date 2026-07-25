# Managed connectors

Managed connectors are the user-friendly layer above universal ingress. A deployment administrator configures provider applications once; end users choose a branded service card, authorize the account, select a Dispatch recipient, and test the connection. End users are never asked for API keys, bot tokens, client IDs, or client secrets.

## Lifecycle

```text
catalog → connect/authorize → credential verification → activation
   → provider event → normalization → durable Dispatch notification
   → test / pause / reconnect / disconnect
```

The common core owns connection state, AES-GCM credential storage, OAuth state and PKCE, provider timeouts, activation errors, sample notifications, deduplication, and lifecycle controls. Provider adapters only define authorization, credential checks, subscription transport, and event normalization.

## Current capability matrix

| Connector | User flow | Event transport | Current state |
|---|---|---|---|
| Demo | Use **Test Dispatch** and select a recipient | Internal sample | Fully available; no account required; shown outside the service catalog |
| Telegram | Enter phone number, Telegram code, and optional 2FA password | MTProto user session | Personal account authorization, encrypted session persistence, worker-managed real-time private/group/channel message ingestion |
| Discord | Install the configured bot | Gateway WebSocket | Existing bridge supported; deployment must configure the Discord application and bridge |
| GitHub | Install a deployment-owned GitHub App | GitHub App webhook | Catalog/install flow; deployment must create and configure the GitHub App |
| Google / Gmail | Google consent screen | OAuth 2.0 and Gmail Pub/Sub | OAuth core available; Cloud Pub/Sub topic/watch provisioning remains an action-required deployment step |
| YouTube | Enter a channel ID | WebSub/PubSubHubbub | Subscription activation and Atom event normalization; public HTTPS required |
| Universal webhook | Open **Developer tools** | Authenticated HTTP POST | Fully available for arbitrary JSON services; intentionally outside the consumer catalog |

Provider constraints are taken from their official documentation: [Telegram user authorization](https://core.telegram.org/api/auth), [Discord OAuth and bot installation](https://docs.discord.com/developers/topics/oauth2), [GitHub Apps](https://docs.github.com/en/apps/creating-github-apps), [Gmail push notifications](https://developers.google.com/workspace/gmail/api/guides/push), and [YouTube push notifications](https://developers.google.com/youtube/v3/guides/push_notifications).

Telegram uses the official MTProto user-authorization flow, not a bot. Viber, ChatGPT, and OpenAI API are not shown as consumer integrations because they do not provide the requested personal-notification flow. The outbound Telegram delivery channel remains available and is separate from the inbound Telegram account session.

## Deployment configuration

```dotenv
CONNECTOR_PUBLIC_URL=https://dispatch.example
CONNECTOR_ENCRYPTION_KEY=<independent long random secret>

TELEGRAM_API_ID=
TELEGRAM_API_HASH=
GITHUB_APP_SLUG=
DISCORD_CLIENT_ID=
GOOGLE_OAUTH_CLIENT_ID=
GOOGLE_OAUTH_CLIENT_SECRET=
```

`CONNECTOR_PUBLIC_URL` must be the externally reachable origin with no path suffix. YouTube, provider webhooks, and production OAuth callbacks require valid public HTTPS. Google allows localhost redirect URIs for development, but the exact callback still needs to be registered:

```text
https://dispatch.example/connect/v1/oauth/google/callback
```

`CONNECTOR_ENCRYPTION_KEY` must remain stable for the life of stored connections. If omitted, Dispatch falls back to `INGRESS_ENCRYPTION_KEY`, then the API key for local compatibility. Production should always use an independent value and back it up with the database.

The deployment owner registers one Telegram application at [my.telegram.org/apps](https://my.telegram.org/apps) and stores its API ID and hash in the server environment. End users only enter their phone number, the one-time code delivered by Telegram, and their 2FA password when enabled. The password is never persisted. The resulting MTProto session is encrypted at rest and gives Dispatch the same message access the user approved for this custom client.

## Adding another connector

Most new services require:

1. A manifest in `internal/connectors/catalog.go`.
2. Credential or OAuth validation in the connector service.
3. Activation/deactivation logic for webhook, polling, WebSub, or Gateway transport.
4. A normalizer that emits `NormalizedEvent`.
5. Provider contract tests and an entry in this capability matrix.

Use a declarative manifest when standard OAuth and JSON mapping are sufficient. Add Go adapter code only for provider-specific signatures, non-JSON payloads, subscription APIs, or persistent Gateway sessions.
