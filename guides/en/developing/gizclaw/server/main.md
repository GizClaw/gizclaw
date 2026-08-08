# Server

`Implementation file: server.go`

Defines a reusable `Server` composition root: receive identity, Peer listeners, explicit Store capabilities, and runtime configuration; initialize services; start HTTP and Peer listeners; process Peer events; and manage shutdown. It has no Store-name or prefix fallback.

It can combine multiple fields, but single field resource, validation, storage and lifecycle should stay in `services/<domain>`. Process configuration and startup belong to `cmd/internal/server`.

## Storage, Store, and service composition

Server configuration has three distinct layers:

| Layer | Shape | Ownership |
| --- | --- | --- |
| `storage` | dynamic map | Physical connection, credentials, pool/client construction, readiness, and close |
| `stores` | dynamic map | Logical interface kind and prefix/table/topic scope |
| `services` | fixed typed structure | Built-in service fields that name compatible Stores |

Registry names are exact and case-sensitive. The Server never assigns meaning to names such as `peers` or `metrics`; every built-in consumer uses a fixed `services` field. Core service blocks are required. `services.agent_host`, `services.metrics`, and `services.system_log` are optional. Omitted SystemLog selects info-level stderr.

The main capability groups are:

| Service fields | Capability |
| --- | --- |
| Peer, login, credential, firmware, RuntimeProfile, model, voice, MemoryLayout, provider tenants, workflow, toolkit, contact, friend, and Friend Group | one `keyvalue` each; code owns internal collection prefixes |
| `services.workspace.assets_store`, `services.gameplay.assets_store`, `services.agent_host.runtime_store` | `objectstore` |
| `services.gameplay.database_store` | `sql` |
| `services.agent_host.flowcraft.history_store` | `log.mutable` |
| `services.metrics.store` | `metrics` |
| `services.system_log.query_store` and Store sinks | immutable Log capability |

Friend Group groups, invite tokens, members, and belongs are code-owned scopes over one Service Store, so they share one atomic KV transaction boundary. Shared ObjectStores require non-empty, clean, non-overlapping prefixes. Missing references, wrong kinds, immutable Flowcraft History, and unknown fields fail before listeners open.

Startup strictly parses the configuration, opens physical connectors, builds logical Stores, resolves service capabilities, lets active SQL-backed services validate their schemas, installs logging and metrics, and only then opens listeners. Logical Stores never close borrowed connectors. Shutdown closes logical wrappers before physical connectors.

The old one-layer Store configuration, top-level pseudo-service blocks, implicit Store names, generic `kind: log`, and `gizclaw migrate` command are unsupported. Recreate development workspaces with the current configuration; no old data is imported or transformed. Gameplay and ClickHouse Store table initialization remain active schema lifecycle, not old-data migration.

### Complete configuration

The following development deployment uses Badger for KV state, SQLite for Gameplay SQL, a filesystem ObjectStore, Volc TLS logs, and one ClickHouse pool shared by Metrics and Flowcraft History. Production can change `storage.gameplay-db.kind` to `postgresql` and replace `dir` with `dsn` without changing the logical Store or service binding.

<<< ../../../../snippets/server-storage-stores-services.yaml{yaml}

`memory.Store` remains selected by RuntimeProfile and MemoryLayout; `stores.kind: memory` is invalid. Physical `storage.kind: memory` only provides an in-process backend to compatible keyvalue or metrics Stores. See [Memory Store](/en/developing/stores/memory).

Each Workspace Agent generation resolves its memory alias from the current RuntimeProfile snapshot. Construction failure fails Agent initialization or reload explicitly. Server shutdown closes the shared Memory registry. Workspace reload and release of the final Agent reference close that generation's lease without migrating, merging, copying, or deleting durable data.

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
