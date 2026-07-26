# Threat model

| Asset | Threat | Control |
|---|---|---|
| API and message data | Unauthorized access | Trusted-local mode by default; deployments exposed beyond localhost must enable constant-time API-key authentication, use a CORS allowlist, request limits, rate limits, and TLS |
| Ingress endpoints | Forged or replayed events | Per-source credentials, raw-body signature verification, constant-time comparisons, timestamp windows for Slack/Stripe, source-scoped event deduplication, global request size/rate limits |
| Ingress signing secrets | Database disclosure | AES-GCM encryption under a deployment key separate from the database; bearer/custom-header secrets are stored only as hashes; secrets shown once |
| Connector credentials | Token disclosure or confused OAuth callback | AES-GCM credential envelope, credentials never returned by list APIs, expiring single-use OAuth state, PKCE, exact redirect URI, provider timeouts, no token/payload logging |
| Telegram user session | Account takeover after database disclosure | MTProto session encrypted under the connector deployment key, phone/code authorization attempts expire after ten minutes, 2FA passwords are never stored, session bytes never appear in APIs or logs |
| Connector events | Forged or duplicate provider events | Provider webhook signatures where available, OAuth-bound polling, connector/event unique keys, global body and rate limits |
| Connector lifecycle | Orphaned external subscriptions | Deactivation before disable/delete, visible action-required/error states, explicit retry and sample-delivery controls |
| Discord bridge | Untrusted users or channels | Mandatory user allowlist, optional channel allowlist, bot-message rejection, privileged Message Content intent disabled by default, payload-free logs |
| Provider credentials | Disclosure | Environment-only configuration, settings expose readiness only, log messages omit payloads and secrets |
| Internal network | Webhook SSRF | HTTP(S)-only parsing, no URL credentials/fragments, DNS resolution, private/loopback/link-local blocking |
| Queue integrity | Duplicate or lost work | Atomic enqueue, unique idempotency keys, row locks with `SKIP LOCKED`, stale lock recovery |
| Delivery providers | Infinite retry amplification | Bounded exponential schedule, retryability classification, dead-letter terminal state |
| AI analysis | Prompt injection or invalid AI output | Metadata allowlist, instructions separated from untrusted content, structured schema, validation, confidence gate, deterministic fallback, and no AI control over destinations |
| Database | Injection | Parameterized pgx queries and fixed filter clauses |

Residual risk: GitHub-compatible HMAC does not include a trusted timestamp, so replay protection depends on the provider delivery ID and Dispatch deduplication. At-least-once processing may repeat an external side effect if a delivery provider accepts a request but its response is lost. Provider idempotency identifiers and reconciliation are required.

`CONSOLE_AUTH_ENABLED=false` intentionally leaves the administrative API open. It is suitable only for local testing or a separately authenticated private network and is not a piracy-control mechanism.
