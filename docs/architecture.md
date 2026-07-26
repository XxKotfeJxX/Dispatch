# Architecture

```text
Browser / API producer / provider account
        |            |             |
  connector UI    OAuth/App    webhook/Gateway/polling
        |            |             |
        +-------- connector core --+
                     |
              universal normalizer
                     |
                  Go API  ---- health / metrics / SSE
        |
    PostgreSQL
   /          \
domain data   durable jobs
                  |
              Go worker
          /       |       \
       SMTP   Telegram   Webhook
                  |
          optional Gemini advice
```

The API and worker are separate processes over the same modular-monolith packages. PostgreSQL is the only coordination dependency.

Notification creation and the first `process_notification` job are committed atomically. Workers claim ready jobs in short transactions using `FOR UPDATE SKIP LOCKED`, release the transaction, perform external I/O with deadlines, then persist the attempt result. A stale lock recovery pass requeues jobs abandoned by an interrupted worker.

Universal ingress authenticates a source-specific endpoint, maps nested JSON fields to the notification contract, and derives the idempotency key from the source plus its external event ID. Presets provide provider-specific authentication and mappings without creating separate delivery pipelines.

Managed connectors add a lifecycle around that boundary. Static manifests describe provider capabilities and setup requirements; persisted connections contain non-secret configuration plus AES-GCM encrypted credentials. OAuth callbacks consume single-use, expiring state records and PKCE verifiers. Provider-specific callbacks normalize into the same notification and job pipeline, so connector code does not implement delivery.

The recipient selected by a connector or ingress source determines the delivery destination. Its default channel, disabled-channel preferences, destination presence, provider readiness, scheduling, and quiet hours are authoritative. Dispatch never silently reroutes a notification to a different recipient channel.

Gemini is an optional analysis step. It receives bounded notification text plus allowlisted metadata and returns a schema-validated category, priority, factual same-language summary, confidence, and controlled reason codes. A valid decision above the confidence threshold may set category, priority, and summary, but it cannot select or change a delivery destination. The Settings console exposes the active non-secret profile and a live analyzer preview.

The self-hosted analysis profile is configured once for the deployment:

```dotenv
AI_ENABLED=true
GEMINI_API_KEY=<Google AI Studio API key>
GEMINI_MODEL=gemini-3.5-flash-lite
AI_TIMEOUT=12s
AI_MIN_CONFIDENCE=0.75
AI_MAX_INPUT_CHARS=12000
AI_MAX_OUTPUT_TOKENS=512
AI_SUMMARY_MAX_CHARS=180
AI_THINKING_LEVEL=minimal
AI_PROMPT_VERSION=dispatch-analysis-v3
```

`minimal` thinking is intentional for fast, high-volume structured classification. Low-confidence or failed provider responses use the deterministic fallback and never stop notification delivery.

See ADRs for the decisions behind [PostgreSQL jobs](adr/0001-postgres-job-queue.md) and [advisory AI](adr/0002-advisory-ai-routing.md).
