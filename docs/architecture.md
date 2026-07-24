# Architecture

```text
Browser / producer
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

Routing order is:

1. Explicit requested channels.
2. First matching enabled deterministic rule by priority.
3. Valid Gemini decision above the confidence threshold.
4. Recipient default channels.
5. Eligible webhook fallback.

Recipient preferences, destination presence, provider readiness, disabled channels, scheduling, and quiet hours are applied after the route source is chosen.

See ADRs for the decisions behind [PostgreSQL jobs](adr/0001-postgres-job-queue.md) and [advisory AI](adr/0002-advisory-ai-routing.md).
