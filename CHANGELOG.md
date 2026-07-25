# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [1.2.0] - 2026-07-25

### Added

- Managed connector core with declarative manifests, encrypted credentials, lifecycle state, OAuth state/PKCE, provider tests, activation, pause, disconnect, and sample delivery.
- User-facing Integrations catalog for Telegram, Discord, GitHub, Google/Gmail, and YouTube, with separate Demo and Universal Webhook tools.
- Telegram personal-account authorization through phone code and optional 2FA, encrypted MTProto session persistence, session health checks, and worker-managed real-time message ingestion.
- YouTube WebSub subscription and event normalization, and Google OAuth callback foundation.
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
