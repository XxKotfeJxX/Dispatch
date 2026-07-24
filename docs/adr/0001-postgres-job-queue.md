# ADR 0001: PostgreSQL is the durable job queue

Status: Accepted

Dispatch already requires PostgreSQL for state. Jobs therefore live beside notification records and can be enqueued in the same transaction. Workers claim them with `FOR UPDATE SKIP LOCKED`, use leases for crash recovery, and persist bounded retries.

This avoids a second distributed system while satisfying v1 throughput and durability. Kafka and Redis are not introduced. If measured contention exceeds the database design envelope, a future ADR may revisit this decision.
