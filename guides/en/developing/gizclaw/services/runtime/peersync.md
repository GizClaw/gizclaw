# Peer Sync

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peersync)

`peersync` owns state synchronization checkpoints for an owner Peer. Domain
services retain resource ownership, authorization, and read projections. Peer
HTTP composes the existing reads into the finite SSE response defined by the
[Public API](../../../api/http/public#peer-state-synchronization). It does not
subscribe to device events or retain device connections.

Checkpoints store SHA-256 hashes of JSON projections under canonical resource
paths, without retaining resource payloads. Incremental synchronization compares
the last completed checkpoint with current projections, returning complete
upserts and deletes for missing items. An absent baseline produces a reset and
full state. Clients stage the response and commit state and timestamp on done.

The Server reuses `services.peer.store`, addressing
`peer-sync/<owner>/head` and `peer-sync/<owner>/checkpoints/<timestamp>` exactly.
KV `ApplyMutation` atomically compares the head, saves the new checkpoint, and
removes evicted checkpoints without enumerating database keys. Each owner retains
at most 64 checkpoints for at most 24 hours; the head also expires. Timestamps
increase monotonically per owner even within one millisecond or after clock
rollback, and remain JavaScript safe integers. Conditional writes use bounded
retries; cancellation and storage failure produce no successful completion event.

This service owns no workers, lock tables, or subscriptions to close. Persistence
belongs to the configured KV backend. Losing a memory store naturally causes old
timestamps to receive a full reset.

## Giztest validation

`server.peer.sync.giztest.yaml` verifies synchronization through real Edge, Server, WebRTC, and API keys. Ordinary Go CI runs `TestPeerSyncGiztest`; JavaScript/native Flutter execute the same scenario. See [Testing and E2E](../../../testing) for commands and coverage.
