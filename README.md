# Dispatch

Dispatch is a self-hosted notification orchestration service. It accepts events through managed connectors, its API, or authenticated universal ingress, chooses eligible channels through deterministic policy with optional Gemini advice, and delivers through email, Telegram, or webhooks with durable scheduling, retries, and a complete audit trail.

The product is a single-tenant modular monolith: a Go API and worker share PostgreSQL as the source of truth, while a React operations console exposes routing and delivery state.

## Quick start

Requirements: Docker Engine with Compose.

```bash
docker compose up --build
```

Open:

- Console: http://localhost:8090
- Mailpit: http://localhost:8025
- API health: http://localhost:8090/healthz

The development API key is `dispatch-local-development-key`. Replace it in `.env` before exposing Dispatch outside localhost.

Create a notification:

```bash
curl -X POST http://localhost:8090/api/v1/notifications \
  -H "Content-Type: application/json" \
  -H "X-API-Key: dispatch-local-development-key" \
  -d '{
    "idempotency_key": "demo-001",
    "recipient_id": "rec_demo_ops",
    "event_type": "payment.failed",
    "subject": "Payment needs attention",
    "body": "Order 42 could not be charged.",
    "metadata": {"order_id": "42", "environment": "production"}
  }'
```

## Guarantees

- PostgreSQL-backed durable jobs claimed with `FOR UPDATE SKIP LOCKED`.
- At-least-once processing and channel-level idempotency identifiers.
- Notification creation and initial job enqueue in one transaction.
- Explicit routing and deterministic rules take precedence over AI.
- AI output is schema-validated; timeout, invalid output, low confidence, or disabled AI uses deterministic fallback.
- Bounded retries end in an inspectable dead-letter state.
- Webhook destinations reject loopback, private, link-local, and unsafe URLs by default.
- Ingress sources use per-source bearer, custom-header, HMAC, Slack, or Stripe authentication and payload-level deduplication.
- Managed connections share encrypted credentials, OAuth state/PKCE, health tests, pause/disconnect controls, and sample delivery.
- Secrets are environment-only and never returned by the settings API.

## Configuration

Copy `.env.example` to `.env`. Local console access does not require a key by default. Before exposing Dispatch outside a trusted local network, set `CONSOLE_AUTH_ENABLED=true` and a unique `API_KEY` of at least 16 characters. Set a stable, independent `INGRESS_ENCRYPTION_KEY` before creating signed sources. AI is off by default. To enable advisory routing, set `AI_ENABLED=true` and `GEMINI_API_KEY`. The default stable model is `gemini-3.5-flash-lite`.

For local webhook targets, `WEBHOOK_ALLOW_PRIVATE=true` is an explicit development-only escape hatch.

Open **Integrations** in the console for the connector catalog and one-click test flow. Telegram, Discord, Viber, GitHub, Google, YouTube, Demo, and the Universal Webhook fallback expose their real deployment requirements before connection. See [Managed connectors](docs/connectors.md) and [Universal ingress](docs/ingress.md).

## Development

```bash
make test
make build
cd web && pnpm install && pnpm test && pnpm build
```

Architecture and operational guidance live in [`docs/architecture.md`](docs/architecture.md), [`docs/connectors.md`](docs/connectors.md), [`docs/ingress.md`](docs/ingress.md), [`docs/runbook.md`](docs/runbook.md), and [`docs/threat-model.md`](docs/threat-model.md). The API contract is [`docs/openapi.yaml`](docs/openapi.yaml).

## Project status

Dispatch v1.2 adds the managed connector platform. Redis, Kafka, Kubernetes, multi-tenancy, SMS, and alternate AI providers remain intentionally out of scope.

Licensed under the MIT License.
