# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [1.2.0] - 2026-07-25

### Added

- Managed connector core with declarative manifests, encrypted credentials, lifecycle state, OAuth state/PKCE, provider tests, activation, pause, disconnect, and sample delivery.
- User-facing Integrations catalog for Demo, Telegram, Discord, Viber, GitHub, Google/Gmail, YouTube, ChatGPT capability status, and Universal Webhook.
- Telegram and Viber bot verification/webhook adapters, YouTube WebSub subscription and event normalization, and Google OAuth callback foundation.
- Connector persistence, event deduplication, migration, documentation, threat controls, and end-to-end UI coverage.

### Changed

- Manual webhook Sources moved under Integrations as an advanced fallback instead of the primary setup flow.
- Local console access no longer asks for an API key by default; exposed deployments can opt into the legacy single-key protection with `CONSOLE_AUTH_ENABLED=true`.

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
