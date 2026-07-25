# Managed connectors

Managed connectors are the user-friendly layer above universal ingress. A deployment administrator configures provider applications once; end users then choose a service, authorize or paste the minimum provider credential, select a Dispatch recipient, and test the connection.

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
| Demo | Select recipient and connect | Internal sample | Fully available; no account required |
| Telegram | Paste a BotFather bot token once | Bot API webhook | Token verification and automatic webhook registration; public HTTPS required for live events |
| Discord | Install the configured bot | Gateway WebSocket | Existing bridge supported; deployment must configure the Discord application and bridge |
| Viber | Paste a commercially provisioned bot token | Bot API webhook | Technically supported, but Viber bot creation is commercial |
| GitHub | Install a deployment-owned GitHub App | GitHub App webhook | Catalog/install flow; deployment must create and configure the GitHub App |
| Google / Gmail | Google consent screen | OAuth 2.0 and Gmail Pub/Sub | OAuth core available; Cloud Pub/Sub topic/watch provisioning remains an action-required deployment step |
| YouTube | Enter a channel ID | WebSub/PubSubHubbub | Subscription activation and Atom event normalization; public HTTPS required |
| ChatGPT | — | — | No official consumer ChatGPT-notification subscription API; intentionally unavailable |
| Universal webhook | Advanced builder | Authenticated HTTP POST | Fully available for arbitrary JSON services |

Provider constraints are taken from their official documentation: [Telegram Bot API](https://core.telegram.org/bots/api), [Discord OAuth and bot installation](https://docs.discord.com/developers/topics/oauth2), [Viber Bot API](https://developers.viber.com/docs/api/rest-bot-api/), [GitHub Apps](https://docs.github.com/en/apps/creating-github-apps), [Gmail push notifications](https://developers.google.com/workspace/gmail/api/guides/push), and [YouTube push notifications](https://developers.google.com/youtube/v3/guides/push_notifications).

OpenAI's [Apps SDK](https://developers.openai.com/apps-sdk/) exposes external tools to ChatGPT; it is the opposite direction from exporting a user's ChatGPT notifications. Dispatch must not present an authorization button until an official API exists for the requested direction.

## Deployment configuration

```dotenv
CONNECTOR_PUBLIC_URL=https://dispatch.example
CONNECTOR_ENCRYPTION_KEY=<independent long random secret>

GITHUB_APP_SLUG=
DISCORD_CLIENT_ID=
GOOGLE_OAUTH_CLIENT_ID=
GOOGLE_OAUTH_CLIENT_SECRET=
```

`CONNECTOR_PUBLIC_URL` must be the externally reachable origin with no path suffix. Telegram, Viber, YouTube, provider webhooks, and production OAuth callbacks require valid public HTTPS. Google allows localhost redirect URIs for development, but the exact callback still needs to be registered:

```text
https://dispatch.example/connect/v1/oauth/google/callback
```

`CONNECTOR_ENCRYPTION_KEY` must remain stable for the life of stored connections. If omitted, Dispatch falls back to `INGRESS_ENCRYPTION_KEY`, then the API key for local compatibility. Production should always use an independent value and back it up with the database.

## Adding another connector

Most new services require:

1. A manifest in `internal/connectors/catalog.go`.
2. Credential or OAuth validation in the connector service.
3. Activation/deactivation logic for webhook, polling, WebSub, or Gateway transport.
4. A normalizer that emits `NormalizedEvent`.
5. Provider contract tests and an entry in this capability matrix.

Use a declarative manifest when standard OAuth and JSON mapping are sufficient. Add Go adapter code only for provider-specific signatures, non-JSON payloads, subscription APIs, or persistent Gateway sessions.
