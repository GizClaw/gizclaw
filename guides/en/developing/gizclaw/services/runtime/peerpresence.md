# Shared presence

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence)

`peerpresence` stores cross-Server connection state, the last observed activity and the running Workspace in the `presence` namespace of `services.peer.store`. Friend and member lists read these snapshots after relationship authorization. Queries never read `services.peer_run.store` SQL. Local connections, persisted run selection and runtime history retain their existing ownership.

## Publication and reads

Manager publishes online, offline, replacement, retirement and running-Workspace transitions. Agent Host notifies it after starting, running, stopping, stopped and error transitions; only running state carries a Workspace. Queries, audio traffic and telemetry do not publish presence. `last_seen_at` is activity observed at the last transition, rather than a continuously refreshed clock.

Publication queues a snapshot without storage I/O under Manager or Agent Host state locks. Each Peer has an independent serial writer that coalesces intermediate pending states. Each operation carries a two-second context deadline; failed writes retry the latest state after one second until successful or closed. Different Peers can progress independently. Redis clients opened by Storage apply context deadlines to socket I/O; Go callers borrowing their own clients should enable that option too. Queries use ordinary KV reads: Redis read/write separation uses the reader, while writes and atomic conditions use the writer. Read errors return offline without a last-seen time and never fall back to SQL or the primary.

Manager gives every accepted connection its own `Publisher`. Initial online publication reads `owners/<peer>` at the write authority and atomically claims a token with its snapshot. Later Workspace and offline updates must match that token. After another Server or a replacement connection claims it, late updates from the old connection cannot overwrite its state; other Peers keep publishing. The Publisher retains its claim condition and fencing for the connection's entire lifetime, so deletion rollback cannot reclaim ownership after an offline worker retires. The initial claim condition survives retries, so a lost commit response cannot reclaim another connection's ownership. A connection coalesced into offline before any online attempt claims no token and preserves existing history. An online commit with a lost response is revoked only while its token still belongs to that connection. Tokens have no per-Peer heartbeat; readers still fetch only snapshots and Server leases.

Server composition uses the existing Peer KV automatically. Servers must share that physical namespace; sharing only Social KV does not share presence. Ordinary Redis, `redis.readwrite` and existing KV backends retain their configuration and require no additional service binding.

List pages use `ReadBatch` for Peer snapshots, followed by one lease per distinct Server. Both rounds share a two-second presence budget, including the optional group Workspace binding lookup. Shorter caller deadlines win. Authorization, pagination and profile lookups retain the request context. Results preserve page order; individual failures report offline without a last-seen time while successful independent snapshots remain available.

Bulk I/O uses the required [KV](/en/developing/stores/kv) method `Store.BatchGet`. TiKV uses one historical snapshot and native SDK BatchGet per batch, Redis uses native `MGET` on the selected reader, and other backends own at most eight concurrent point reads. Peer and lease rounds do not share a snapshot. Legacy Go providers exposing only single-Peer presence retain at most eight concurrent reads.

## Server lease

Each Server boot has a unique generation and renews one Server lease every ten seconds, without per-Peer heartbeats. Its default lifetime is thirty seconds. TiKV historical reads add the configured `kv.ReadDelay` window to that lifetime so healthy leases remain valid in historical snapshots. Historical reads still delay visibility of initial connections and later transitions.

Reads validate the snapshot generation, lease and absolute expiry time. A crashed Server, failed renewal or expired lease makes its Peers offline; graceful shutdown revokes the lease. A new generation fences writes, renewal and lease revocation from older processes. Servers should keep their clocks synchronized. Redis lag exceeding the lease window conservatively reports offline.

Close cancels and joins all workers without closing the borrowed KV Store. Presence is display data, not relationship authority, route ownership or delivery evidence. Notification delivery still requires the target device ACK.

## Validation

`go test -race ./pkgs/gizclaw/services/runtime/peerpresence ./pkgs/gizclaw` covers expired leases, replaced Server generations, late Workspace/offline updates during cross-Server handoff, lost claim responses, disconnects, runtime transitions and cross-Manager queries with unavailable SQL. Batch tests cover lease deduplication, corruption, unknown Peers and a shared deadline across both read rounds. `tests/store-e2e/run_redis_readwrite_tests.sh` exercises real Redis primary/replica behavior. `tests/store-e2e/run_tikv_tests.sh <evidence-dir> --presence-only` verifies real Learner reads, replication lag and stalled TCP responses.
