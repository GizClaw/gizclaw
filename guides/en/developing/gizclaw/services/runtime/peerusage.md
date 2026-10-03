# Peer model usage

`pkgs/gizclaw/services/runtime/peerusage` owns hourly Peer consumption, a nonblocking recorder, background writes and 90-day retention.

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerusage)

## Configuration and attribution

```yaml
stores:
  peer-usage:
    kind: sql
    storage: database
services:
  peer_usage:
    store: peer-usage
```

The connector must be SQLite or PostgreSQL. Omitting `services.peer_usage` disables persistent metering. Configured schemas are validated before listeners start; the service borrows and never closes the pool.

Connected Peer calls, standalone OpenAI HTTP calls and Workspace-owner provider services install a recorder around actual provider calls. Attribution uses the calling Peer or Workspace owner's public key. `model_id` is the provider's billing model/version/resource ID; TTS uses its billing model/resource, not a Voice ID. Disjoint `Input`, `CachedInput` and `Output` quantities are added without persisting modality, prices, unit conversion or call logs.

## Table schema

The fixed parent is `peer_model_usage_hourly`.

| Column | Type | Meaning |
| --- | --- | --- |
| `peer_public_key` | TEXT | Peer identity; deletion does not cascade into usage |
| `model_id` | TEXT | Provider billing model/version/resource ID |
| `hour_unix_nano` | BIGINT | UTC hour boundary as Unix nanoseconds |
| `writer_id` | TEXT | Process/bucket writer epoch for idempotent retries |
| `quantity` | BIGINT | Nonnegative cumulative quantity for that writer/hour |

The primary key is `(peer_public_key, model_id, hour_unix_nano, writer_id)`. A cumulative snapshot only increases stored quantity. Retrying an acknowledged or ambiguously committed snapshot does not add it twice. Writers contribute independently; grouped reads sum quantity by model/hour for the selected Peer. There is no separate cumulative-total table or request journal.

## Hour buckets, day partitions and retention

Hours are the aggregation unit; PostgreSQL uses UTC day range partitions on `hour_unix_nano`. Children are named `peer_model_usage_hourly_pYYYYMMDD`. Callers read/write the parent and PostgreSQL routes rows. Initialization and writes prepare required/following days and drop children wholly before the retention boundary. The shared `storage.SQLDailyPartitions` capability is also used by LogStore; its expiry-based partition key and auxiliary-key cleanup remain unchanged.

The boundary is the current UTC hour minus 90 days. Queries immediately hide older hours. Physical PostgreSQL deletion works at day granularity; SQLite deletes expired rows. An hourly idle maintenance tick reclaims data even without new reports. There is no partition-registration table or per-Peer table.

## Concurrency and durability

Provider callbacks only coalesce in-memory hourly counts and perform no database/network I/O. The worker writes cumulative snapshots once per second and retains unacknowledged snapshots after failure. Server shutdown cancels and joins the worker before a final bounded 30-second flush and before borrowed pools close. Unsuccessful persistence is returned as an error.

Only successfully flushed quantities are durable. Unclean process termination can lose the one-second volatile window or pending quantities during a database outage. This recorder does not provide crash-proof billing, external authorization, quota exchange or expiry enforcement.
