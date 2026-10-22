# Peer model usage

`pkgs/gizclaw/services/runtime/peerusage` owns hourly Peer consumption, asynchronous persistence and retention.

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

## Storage and queries

Usage is aggregated by Peer, provider model, and time window. Queries read those aggregates rather than a per-request log or a second total table. Retry idempotency, SQL schemas, partitions, and retention are owned by [store.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerusage/store.go).

## Durability and shutdown

Provider callbacks hand quantities to an in-memory recorder; a worker persists them. Drain the recorder before closing its borrowed SQL pool. See [recorder.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerusage/recorder.go).

Only successful writes are durable. Crashes or database failures can lose unconfirmed quantities; this recorder supports operational statistics and does not replace a billing system.
