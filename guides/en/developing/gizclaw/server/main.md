# Server

`Implementation file: server.go`

Defines a reusable `Server` composition root: receive identity, Peer listeners, explicit Store capabilities, and runtime configuration; initialize services; start HTTP and Peer listeners; process Peer events; and manage shutdown. It has no Store-name or prefix fallback.

It can combine multiple fields, but single field resource, validation, storage and lifecycle should stay in `services/<domain>`. Process configuration and startup belong to `cmd/internal/server`.

## Server info and build identity

`version` and `build_commit` in `/server-info` come from metadata embedded when the binary is built. A formal Release uses `MAJOR.MINOR.PATCH` without a leading `v`, while an uninjected local development build reports `dev`. Release builds must inject both the version and the full source commit; runtime configuration cannot override the software version.

`public_key` remains the authoritative Server's only identity. An Edge rewrites only transport routing and does not change the Server's `public_key`, version, or build commit.

## Storage, Store, and service composition

Server configuration has three distinct layers:

| Layer | Shape | Ownership |
| --- | --- | --- |
| `storage` | dynamic map | Physical connection, credentials, pool/client construction, readiness, and close |
| `stores` | dynamic map | Logical interface kind and prefix/table/topic scope |
| `services` | fixed typed structure | Built-in service fields that name compatible Stores |

Registry names are exact and case-sensitive. The Server never assigns meaning to names such as `peers` or `metrics`; every built-in consumer uses a fixed `services` field. Core service blocks are required. `services.agent_host`, `services.metrics`, `services.system_log`, and `services.sfu` are optional. Omitted SystemLog selects info-level stderr; omitting `services.sfu` disables Friend and Friend Group SFU Workspaces on this Server.

Global and per-sink `services.system_log` levels also determine whether a newly
accepted Edge-routed logical Peer constructs its complete Server lifecycle
observer. All sinks must reject `INFO` to remove the connection, per-turn, and
Agent-runtime tracking work; one Info-enabled sink keeps the observer enabled.
The decision is fixed for that connection. There is no lifecycle-specific
configuration field, and Server logging configuration does not control the
separate Edge process.

The main capability groups are:

| Service fields | Capability |
| --- | --- |
| `services.peer.store` | `keyvalue`; owns shared Peer records and fixed Server routes |
| `services.peer_run.store` | distinct `keyvalue`; owns Server-local Peer status and Agent selection state |
| login, credential, firmware, RuntimeProfile, model, voice, MemoryLayout, provider tenants, workflow, toolkit, contact, friend, and Friend Group | one `keyvalue` each; code owns internal collection prefixes |
| `services.workspace.history_assets_store`, `services.workspace.assets_store`, `services.gameplay.assets_store`, `services.agent_host.runtime_store` | `objectstore` |
| `services.gameplay.database_store` | `sql` |
| `services.workspace.history_store` | `log.mutable` |
| `services.agent_host.flowcraft.history_store` | `log.mutable` |
| `services.metrics.store` | `metrics` |
| `services.sfu` | references no Store; `url` is the LiveKit `ws://`/`wss://` signaling URL, `api_key_file` and `api_secret_file` are read at startup, `recheck_interval` and `reconnect_timeout` are optional; see [services/social](/en/developing/gizclaw/services/social#configuration) |
| `services.system_log.query_store` and Store sinks | immutable Log capability |

Friend Group groups, invite tokens, members, and belongs are code-owned scopes over one Service Store, so they share one atomic KV transaction boundary. Shared ObjectStores require non-empty, clean, non-overlapping prefixes. Missing references, wrong kinds, immutable Flowcraft History, and unknown fields fail before listeners open.

In a multi-Server topology, all Servers may bind Peer, Friend, and Friend Group control-plane Stores to distinct prefixes on one Redis 7.0 connector. `services.peer_run.store` remains mandatory and distinct from `services.peer.store`, but it may use any supported `keyvalue` backend; the Server retains its code-owned `runs` namespace below that binding. Client activation atomically claims an unassigned Peer or verifies the existing fixed Server owner before publishing the connection or starting service work. RuntimeProfile, Workspace, History, Memory, assets, and other runtime Stores remain Server-local; this composition does not provide Workspace routing or failover.

Startup strictly parses the configuration, opens physical connectors, builds logical Stores, resolves service capabilities, lets active SQL-backed services validate their schemas, and installs logging and metrics. After workspace PID ownership is acquired, optional process profiling resolves its dedicated ObjectStore and publishes a baseline; only then are public listeners opened and `Server.Listen` started. Logical Stores never close borrowed connectors. Shutdown joins the profiling worker before closing logging, logical wrappers, and physical connectors. Process profiling belongs to `cmd/internal/server`; the reusable `pkgs/gizclaw.Server` has no pprof dependency.

The old one-layer Store configuration, top-level pseudo-service blocks, implicit Store names, generic `kind: log`, and `gizclaw migrate` command are unsupported. Recreate development workspaces with the current configuration; no old data is imported or transformed. Gameplay and ClickHouse Store table initialization remain active schema lifecycle, not old-data migration.

### Complete configuration

The complete production configuration below lets prefix-scoped KV, table-scoped Metrics, immutable/mutable Log, and Gameplay raw SQL Stores borrow one PostgreSQL pool while ObjectStores use a separate filesystem root. The SQL backend maps each KV prefix directly to its physical table; the KV declaration has no `table` field. Every logical SQL Store directly ensures its business table and indexes, then validates the exact schema before listeners start; no version or history table is created. Any failure stops startup without a backend fallback. A local SQLite deployment keeps the same `stores` and `services` and changes only `storage.database` to `kind: sqlite` with `dir` or `dsn`.

<<< ../../../../snippets/server-storage-stores-services.yaml{yaml}

`memory.Store` remains selected by RuntimeProfile and MemoryLayout; `stores.kind: memory` is invalid. Physical `storage.kind: memory` is a stateless marker, and every compatible keyvalue or metrics Store creates an independent in-process backend. See [Memory Store](/en/developing/stores/memory).

Each Workspace Agent generation resolves its memory alias from the current RuntimeProfile snapshot. Construction failure fails Agent initialization or reload explicitly. Server shutdown closes the shared Memory registry. Workspace reload and release of the final Agent reference close that generation's lease without migrating, merging, copying, or deleting durable data.

## Workspace reward lifecycle

Starting the Workspace reward dispatcher validates Gameplay SQL schema and the
single activation boundary, then starts its bounded due-work poller. It never
enumerates or reads persisted Workspace records or History during Server
startup, and it has no periodic Workspace catalog scan. A successfully
published AgentHost runtime schedules lazy catch-up for that exact Workspace;
post-History-append notifications are optional latency hints and may be dropped.
Durable pending, retry, and expired-claim windows resume directly from Gameplay
SQL after restart.

## Pending-deletion processor

Server initialization validates the pending-deletion source and handler registry without starting background work. `Server.Listen` starts one immediate scanner plus a bounded worker pool; `Server.Close` cancels scans and active attempts, waits for all processor goroutines, and only then lets command-layer stores close. The durable domain store is the queue source of truth. Producer wake signals and the in-memory dispatch channel only reduce latency, so startup and periodic scans recover committed work after a restart or a dropped signal.

The first production registration is `source=gameplay`, advertising only `kind=pet`, and it exists only when `GameplayDB` is configured. Peer, Workspace, and Friend Group are not registered without their domain handlers. An invalid, duplicate, incomplete, or capability-incompatible registration fails initialization.

The optional top-level `pending_deletion` configuration defaults to `scan_interval: 30s`, `page_size: 100`, `dispatch_capacity: 256`, `workers: 4`, `lease_duration: 2m`, `attempt_timeout: 90s`, `retry_initial: 5s`, `retry_max: 30m`, and `max_attempts: 10`. Durations and counts must be positive and bounded, attempt timeout must be shorter than the lease, retry initial must not exceed retry max, and unknown keys fail strict configuration parsing. There is no completion-retention setting.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`Server`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server) | The composition root of GizClaw Server can be reused. |
| [`PeerListenerOptions`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerListenerOptions) / [`PeerListenerFactory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerListenerFactory) | Describe and create Peer listener. |
| [`Server.ServeHTTP`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.ServeHTTP) | Service Server HTTP surface. |
| [`Server.Listen`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Listen) / [`Serve`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Serve) | Create listeners and accept Peer connections. |
| [`Server.PublicKey`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.PublicKey) | Return Server identity public key. |
| [`Server.PeerService`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.PeerService) / [`Manager`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Manager) | Return the assembled Peer service or online Peer Manager. |
| `init` | Initialize stores, domain services, HTTP mux and Peer Runtime. |
| `servePeerListener` | Accepts Peer connections on a single listener. |
| `startCleanup` | Start background resource cleanup. |
| [`Server.Close`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Close) | Stop listeners, background tasks and close Server resources. |
