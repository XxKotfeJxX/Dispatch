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

Update `API_KEY` and provider secrets in the deployment secret store, then restart API and worker containers. The web console key is held only in the current tab's JavaScript memory and is cleared on reload.
