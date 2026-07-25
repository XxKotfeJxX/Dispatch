# Operations runbook

## Health

- `/healthz` proves the API process is alive.
- `/readyz` proves PostgreSQL is reachable.
- `/metrics` exposes HTTP counters in Prometheus text format.
- The dashboard shows retries, dead letters, AI fallback rate, and recent failures.

## Delivery incident

1. Open the notification detail page and inspect deliveries, attempts, and audit events.
2. Confirm provider configuration in Settings without exposing secret values.
3. Resolve the provider or destination problem.
4. Use Retry only for `failed` or `partially_delivered` notifications.
5. Use the delivery ID as the idempotency reference when reconciling a provider.

## Worker interruption

Jobs left `running` longer than `JOB_LOCK_TIMEOUT` are requeued on worker startup. Restart the worker after verifying PostgreSQL health. At-least-once delivery means a provider can receive a repeated request; webhook consumers must honor `Idempotency-Key`.

## Backup and restore

Back up PostgreSQL with `pg_dump` and protect the archive like production message content. Restore into the same or newer supported PostgreSQL major version, then start the API to apply forward migrations before starting workers.

## Key rotation

Local development uses `CONSOLE_AUTH_ENABLED=false` and does not require a console credential. Before exposing Dispatch beyond a trusted local network, set `CONSOLE_AUTH_ENABLED=true`, configure a unique `API_KEY` with at least 16 characters, and restart the API. When enabled, the web console key is held only in the current tab's JavaScript memory and is cleared on reload.

Rotate an enabled `API_KEY` in the deployment secret store and restart the API. Existing browser tabs must enter the new value.

`INGRESS_ENCRYPTION_KEY` protects recoverable signing secrets in PostgreSQL. Back it up together with the deployment secrets and do not casually rotate it. If it changes, rotate every HMAC, Slack, and Stripe source secret and update the corresponding producer.

For one source, use **Sources → Rotate secret**, update the producer immediately, then send a test event. The old value stops working as soon as rotation succeeds. Disable the source first if an atomic producer update is not possible.

## Ingress incident

1. Check the source is enabled and that the producer targets `/ingest/v1/<slug>`.
2. A `401` means the bearer/header credential or raw-body signature is wrong; for Slack and Stripe also check clock synchronization.
3. A `400` means the payload is not JSON or cannot satisfy transformation constraints.
4. A `200` with `created: false` is an intentional duplicate; a `202` created a notification.
5. Inspect source events, then follow the notification ID into its delivery audit trail.
6. For Discord, inspect `docker compose --profile discord logs discord-bridge`; logs deliberately omit message content and credentials.

## Connector incident

1. Open **Integrations** and inspect `connected`, `action required`, `error`, or `disabled`.
2. Use **Test** to verify provider credentials and retry activation. Tests never return the stored credential.
3. `action_required` commonly means `CONNECTOR_PUBLIC_URL` is not public HTTPS or the provider needs deployment-level setup.
4. Use **Send sample** to separate provider ingestion problems from Dispatch routing or delivery problems.
5. Before changing `CONNECTOR_ENCRYPTION_KEY`, disconnect every managed connector. Losing the key makes stored OAuth credentials unrecoverable.
6. Disconnect normally so Dispatch removes YouTube or other managed subscriptions before deleting local state.
