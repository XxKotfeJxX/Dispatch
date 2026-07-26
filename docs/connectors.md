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
| Discord | Authorize and install the deployment-owned bot | OAuth 2.0 and Gateway WebSocket | Managed connection; recipient, Discord user, server, and routing mode are persisted automatically |
| GitHub | Choose notifications, then install a deployment-owned GitHub App | Signed GitHub App webhook | Managed installation, repository selection, event filtering, signature verification, and automatic recipient routing |
| Google | One Google consent screen with selectable modules | OAuth 2.0 and worker polling | Gmail, Calendar/Meet, Drive/Docs/Sheets/Slides activity, Tasks, and optional Workspace Chat |
| YouTube | Sign in with Google once | OAuth 2.0, Data API, and RSS polling | Automatically discovers the user's subscriptions and reports new uploads without a domain or webhook |
| Universal webhook | Open **Developer tools** | Authenticated HTTP POST | Fully available for arbitrary JSON services; intentionally outside the consumer catalog |

Provider constraints are taken from their official documentation: [Telegram user authorization](https://core.telegram.org/api/auth), [Discord OAuth and bot installation](https://docs.discord.com/developers/topics/oauth2), [GitHub Apps](https://docs.github.com/en/apps/creating-github-apps), [Google Workspace APIs](https://developers.google.com/workspace), and [YouTube subscriptions](https://developers.google.com/youtube/v3/docs/subscriptions/list).

Telegram uses the official MTProto user-authorization flow, not a bot. Viber, ChatGPT, and OpenAI API are not shown as consumer integrations because they do not provide the requested personal-notification flow. The outbound Telegram delivery channel remains available and is separate from the inbound Telegram account session.

## Deployment configuration

```dotenv
CONNECTOR_PUBLIC_URL=https://dispatch.example
CONNECTOR_ENCRYPTION_KEY=<independent long random secret>

TELEGRAM_API_ID=
TELEGRAM_API_HASH=
GITHUB_APP_SLUG=
GITHUB_APP_ID=
GITHUB_WEBHOOK_SECRET=
GITHUB_PRIVATE_KEY_BASE64=
DISCORD_CLIENT_ID=
DISCORD_CLIENT_SECRET=
DISCORD_BOT_TOKEN=
DISCORD_MESSAGE_CONTENT_INTENT=false
DISCORD_ACKNOWLEDGE=true
GOOGLE_OAUTH_CLIENT_ID=
GOOGLE_OAUTH_CLIENT_SECRET=
GOOGLE_POLL_INTERVAL=30s
YOUTUBE_POLL_INTERVAL=10m
```

`CONNECTOR_PUBLIC_URL` is the browser-visible origin with no path suffix. Provider webhooks and production OAuth callbacks require valid public HTTPS. Google allows localhost redirect URIs for development, but each exact callback still needs to be registered:

```text
https://dispatch.example/connect/v1/oauth/google/callback
https://dispatch.example/connect/v1/oauth/youtube/callback
```

`CONNECTOR_ENCRYPTION_KEY` must remain stable for the life of stored connections. If omitted, Dispatch falls back to `INGRESS_ENCRYPTION_KEY`, then the API key for local compatibility. Production should always use an independent value and back it up with the database.

Connected accounts expose **Settings** in the console. Users can rename a connection, change its recipient, and edit provider filters without creating another connection. Enabling or removing Google modules opens the Google consent screen again because the required read-only scopes may change; the OAuth callback updates the same stored connection.

Normalized events include common `connection_id`, `connection_name`, and `account_label` metadata. Provider metadata uses stable template aliases such as `sender`, `url`, and the service-specific variables shown by the template editor. Telegram additionally includes sender ID/username/name, chat ID/title/username/type, timestamp, mention/forward/media flags, and message counters when Telegram supplies them.

Compose uses configurable external DNS forwarders for provider APIs:

```dotenv
DISPATCH_DNS_PRIMARY=1.1.1.1
DISPATCH_DNS_SECONDARY=8.8.8.8
```

Override these with DNS servers reachable from the deployment network when public DNS is blocked.

For Google, create one Google Cloud project and one OAuth 2.0 client for the
entire Dispatch deployment. In **APIs & Services → Library**, enable:

- **Gmail API**
- **Google Calendar API**
- **Drive Activity API**
- **Google Tasks API**
- **Google Chat API** only when the optional Chat module will be offered
- **YouTube Data API v3** for the separate YouTube subscriptions card

Then configure **Google Auth Platform**:

1. Set app name to `Dispatch`, provide support and developer contact emails,
   and choose **External** audience for consumer Google accounts.
2. While the app is in **Testing**, add every account that will test Dispatch
   under **Audience → Test users**.
3. Under **Data Access**, declare the read-only scopes for the modules offered
   by the deployment:

```text
openid
https://www.googleapis.com/auth/userinfo.email
https://www.googleapis.com/auth/gmail.readonly
https://www.googleapis.com/auth/calendar.readonly
https://www.googleapis.com/auth/drive.activity.readonly
https://www.googleapis.com/auth/tasks.readonly
https://www.googleapis.com/auth/chat.spaces.readonly
https://www.googleapis.com/auth/chat.messages.readonly
https://www.googleapis.com/auth/youtube.readonly
```

The two Chat scopes and Google Chat API are optional. The YouTube scope is only
requested when the user connects the separate YouTube card. Chat message listing is
primarily intended for Google Workspace accounts; a Chat failure is reported as
a module warning and does not stop the other selected Google modules.

Create an OAuth client of type **Web application** and register the exact
authorized redirect URI:

```text
http://localhost:8090/connect/v1/oauth/google/callback
http://localhost:8090/connect/v1/oauth/youtube/callback
```

For a hosted deployment, replace `http://localhost:8090` with the exact
`CONNECTOR_PUBLIC_URL`. Put the resulting client ID and client secret in
`GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_CLIENT_SECRET`. `Authorized
JavaScript origins` is not needed because Dispatch uses the server-side OAuth
code flow. Restart `api` and `worker`; an end user then clicks the single Google
card, selects modules and a Dispatch recipient, signs in once, and approves
only the read-only scopes required by those modules.

Each module establishes a current cursor on first connection and does not
import old activity. The worker synchronizes at `GOOGLE_POLL_INTERVAL`:

- **Gmail:** new inbox messages with inbox/unread/important filters; up to
  12 KB of text plus attachment names, MIME types, and declared sizes for
  summaries and AI analysis. Attachment contents aren't processed.
- **Calendar + Meet:** created, updated, and cancelled primary-calendar events,
  Meet links, and configurable upcoming reminders.
- **Drive + Docs/Sheets/Slides:** Drive Activity actions such as edits,
  comments, renames, moves, deletions, restores, and permission changes.
- **Tasks:** updated, completed, and deleted tasks across task lists.
- **Chat:** new messages in spaces accessible to the authorized Workspace user.

OAuth access is read-only and cannot send mail or Chat messages, edit calendar
events, modify files, or change tasks. No domain, Pub/Sub topic, public webhook,
or locally installed Google application is required for polling.

Google classifies some Gmail and Chat scopes as restricted. During development,
Testing grants and refresh tokens expire after seven days. A public release for
arbitrary Google accounts requires Google's OAuth verification and can require
a security assessment when restricted-scope data is stored or transmitted.

The YouTube card reuses the same deployment OAuth client but creates a separate
read-only authorization. Dispatch pages through `subscriptions.list?mine=true`,
keeps the channel list synchronized, and polls each public channel feed at
`YOUTUBE_POLL_INTERVAL`. The initial connection establishes current cursors and
does not import old videos. Newly followed channels are discovered automatically;
unfollowed channels are removed from polling. This mirrors new uploads from the
user's subscriptions, not YouTube's private personalized notification inbox or
its per-channel bell recommendation algorithm.

The deployment owner registers one Telegram application at [my.telegram.org/apps](https://my.telegram.org/apps) and stores its API ID and hash in the server environment. End users only enter their phone number, the one-time code delivered by Telegram, and their 2FA password when enabled. The password is never persisted. The resulting MTProto session is encrypted at rest and gives Dispatch the same message access the user approved for this custom client.

For Discord, register this exact OAuth redirect URI:

```text
https://dispatch.example/connect/v1/oauth/discord/callback
```

For a local installation with `CONNECTOR_PUBLIC_URL=http://localhost:8090`, register:

```text
http://localhost:8090/connect/v1/oauth/discord/callback
```

The deployment owner sets the application client ID, client secret, and bot token once. An end user clicks the Discord card, chooses a Dispatch recipient and routing mode, authorizes Discord, and selects a server. Dispatch persists the Discord user and server returned by the authorization flow. The Gateway bridge starts with the normal Compose stack and authenticates to an internal-only API route with a key derived from `CONNECTOR_ENCRYPTION_KEY`; no webhook source, ingress secret, copied user ID, or channel ID is required.

For GitHub, create one GitHub App for the Dispatch deployment and configure:

```text
Setup URL:   https://dispatch.example/connect/v1/github/setup
Webhook URL: https://dispatch.example/connect/v1/github/events
```

Enable the webhook, use a strong webhook secret, select the `application/json` content type, and subscribe only to the events Dispatch should offer. The supported presets cover pushes, pull requests and reviews, issues and comments, discussions, Actions workflow runs and jobs, checks, deployments, and releases. Grant read-only repository permissions for **Actions**, **Checks**, **Contents**, **Deployments**, **Discussions**, **Issues**, and **Pull requests**, then subscribe to the corresponding events.

Keep **Request user authorization (OAuth) during installation** disabled because Dispatch uses the post-install Setup URL. Enable **Redirect on update** so an already-installed App returns to Dispatch after the user changes or confirms repository access; the state from the Dispatch installation URL is preserved through that update flow.

Set `GITHUB_APP_SLUG` and `GITHUB_APP_ID` from the app settings, put the same webhook secret in `GITHUB_WEBHOOK_SECRET`, download a private key, and store the base64-encoded PEM in `GITHUB_PRIVATE_KEY_BASE64`. The private key is used only to verify that the installation returned by GitHub belongs to this App. End users only select a recipient and event preset, choose repositories on GitHub, and return to an already-connected account.

GitHub cannot deliver webhooks directly to `localhost`. Production therefore needs a public HTTPS `CONNECTOR_PUBLIC_URL` used by both the Setup URL and Webhook URL. Local development can instead use the forwarding option below.

For local testing without a domain, create a channel at `https://smee.io`, put its URL in `GITHUB_SMEE_URL`, keep `CONNECTOR_PUBLIC_URL=http://localhost:8090`, use the local Setup URL below, and use the Smee channel as the GitHub App Webhook URL:

```text
Setup URL: http://localhost:8090/connect/v1/github/setup
Webhook URL: https://smee.io/<your-channel>
```

Start the development forwarder with:

```powershell
docker compose --profile github-dev up -d github-smee
```

Smee is intended only for development because the channel payloads are not private. Dispatch still verifies every forwarded payload with `GITHUB_WEBHOOK_SECRET`. Production must use the deployment's own public HTTPS endpoint.

## Adding another connector

Most new services require:

1. A manifest in `internal/connectors/catalog.go`.
2. Credential or OAuth validation in the connector service.
3. Activation/deactivation logic for webhook, polling, or Gateway transport.
4. A normalizer that emits `NormalizedEvent`.
5. Provider contract tests and an entry in this capability matrix.

Use a declarative manifest when standard OAuth and JSON mapping are sufficient. Add Go adapter code only for provider-specific signatures, non-JSON payloads, subscription APIs, or persistent Gateway sessions.
