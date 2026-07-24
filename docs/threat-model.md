# Threat model

| Asset | Threat | Control |
|---|---|---|
| API and message data | Unauthorized access | Constant-time API-key comparison, in-memory-only console credential, CORS allowlist, request size limit, rate limit, TLS deployment guidance |
| Provider credentials | Disclosure | Environment-only configuration, settings expose readiness only, log messages omit payloads and secrets |
| Internal network | Webhook SSRF | HTTP(S)-only parsing, no URL credentials/fragments, DNS resolution, private/loopback/link-local blocking |
| Queue integrity | Duplicate or lost work | Atomic enqueue, unique idempotency keys, row locks with `SKIP LOCKED`, stale lock recovery |
| Delivery providers | Infinite retry amplification | Bounded exponential schedule, retryability classification, dead-letter terminal state |
| Routing | Prompt injection or invalid AI output | Metadata allowlist, instructions separated from untrusted content, structured schema, validation, confidence gate, deterministic precedence and fallback |
| Database | Injection | Parameterized pgx queries and fixed filter clauses |

Residual risk: at-least-once processing may repeat an external side effect if a provider accepts a request but its response is lost. Provider idempotency identifiers and reconciliation are required.
