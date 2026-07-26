# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- Delivery destinations now come exclusively from the recipient selected by an integration; event payloads and AI can no longer override them.
- Gemini now uses a fixed category and reason-code taxonomy, evidence-based priority rules, factual same-language summaries, bounded inputs, structured output, and explicit zero-retention requests.
- Universal ingress no longer asks users to enter delivery-channel names.
- Connected integrations can be renamed, retargeted, and reconfigured in place; Google scope changes update the existing connection after reauthorization.
- Connector metadata and template variables now use consistent sender, link, account, and provider-specific aliases.

### Added

- Settings now shows the active non-secret AI profile and provides a live notification analysis preview.

### Fixed

- Recovered notification jobs can safely resume after the notification has already reached the queued state.
- Telegram notifications now include sender username/name, chat context, timestamps, and available message flags instead of only an opaque peer and message ID.
- Compose can bypass intermittent Docker Desktop DNS forwarding failures for Google and other external provider APIs.
- AI fallback records now preserve specific timeout, provider HTTP, and invalid-output reasons instead of collapsing them into `ai_unavailable`.

### Removed

- Routing rules console, CRUD API, seed data, and worker evaluation.

## [1.2.0] - 2026-07-25

### Added

- Managed connector core with declarative manifests, encrypted credentials, lifecycle state, OAuth state/PKCE, provider tests, activation, pause, disconnect, and sample delivery.
- User-facing Integrations catalog for Telegram, Discord, GitHub, Google/Gmail, and YouTube, with separate Demo and Universal Webhook tools.
- Telegram personal-account authorization through phone code and optional 2FA, encrypted MTProto session persistence, session health checks, and worker-managed real-time message ingestion.
- YouTube OAuth subscription discovery, local feed polling, event normalization, and Google OAuth callback support.
- Connector persistence, event deduplication, migration, documentation, threat controls, and end-to-end UI coverage.

### Changed

- Manual webhook Sources moved under Integrations as an advanced fallback instead of the primary setup flow.
- Local console access no longer asks for an API key by default; exposed deployments can opt into the legacy single-key protection with `CONSOLE_AUTH_ENABLED=true`.
- Connector cards now open an actionable setup guide when deployment credentials are missing, and the Universal Webhook card opens its source builder directly.
- Reworked the consumer catalog as large branded cards with official service logos, hover names, accessible info popovers, and click-to-connect behavior.
- Replaced the Telegram Bot connector with personal-account MTProto authorization; removed Viber, ChatGPT, and OpenAI API from the consumer integration catalog.

### Fixed

- Prevented the Demo connector modal from crashing when a connector has no credential fields.

## [1.1.0] - 2026-07-25

### Added

- Universal authenticated JSON ingress with generic, GitHub, GitLab, Slack, Stripe, Sentry, Grafana, and Discord presets.
- Per-source recipients, channel routing, nested payload mapping, one-time secrets, rotation, disable/delete controls, event history, and deduplication.
- Optional allowlisted Discord Gateway bridge for direct messages and private test channels.
- Sources console, ingress documentation, threat controls, Docker Compose profile, and release binary.

## [1.0.0] - 2026-07-24

### Added

- Durable notification ingestion, scheduling, delivery retries, dead-letter handling, and idempotency.
- Email, Telegram, and SSRF-protected webhook providers.
- Deterministic routing with optional structured Gemini advisory decisions and safe fallback.
- Recipient, template, routing rule, audit, dashboard, health, readiness, metrics, and SSE APIs.
- React operations console, Docker Compose local stack, Mailpit, tests, security automation, and public documentation.
