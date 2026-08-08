# Server

`Implementation file: server.go`

Define a reusable `Server` composition root: receive identity, Peer listener, stores and running configuration; initialize services in various fields; start HTTP and Peer listener; process Peer event; manage background cleanup, shutdown sequence and module store fallback.

It can combine multiple fields, but single field resource, validation, storage and lifecycle should stay in `services/<domain>`. Process configuration and startup belong to `cmd/internal/server`.

## AgentHost store bindings

Server Config binds AgentHost persistence capabilities to logical names from `stores`:

```yaml
agent_host:
  runtime_store: agenthost
  flowcraft:
    state_store: flowcraft-state
    history_store: flowcraft-history
```

The references work with both the layered `storage` plus `stores` layout and the supported one-layer `stores` layout. Backend configuration remains on the referenced Store; `agent_host` never contains a directory, DSN, credential, prefix, scope, or inline backend.

| Field | Required capability | Supported backend |
| --- | --- | --- |
| `agent_host.runtime_store` | `objectstore.ObjectStore` | filesystem ObjectStore |
| `agent_host.flowcraft.state_store` | `kv.Store` | Memory or Badger KV |
| `agent_host.flowcraft.history_store` | `logstore.MutableStore` | ClickHouse LogStore; immutable Volc LogStore is rejected |

`agent_host` is the only source of these bindings. Omitting the whole block or a nested reference disables that optional capability; Store names have no reserved binding semantics. An unknown name, wrong Store kind, immutable History Store, unknown field, or empty reference fails Server construction instead of falling back.

`agent_host.flowcraft.memory_store`, `agent_host.eino.memory_store`, `memory_objects_store`, and `stores.kind: memory` are not valid Server Config. The strict parser rejects those legacy fields. Admin `MemoryLayout` owns Memory policy, while RuntimeProfile `resources.memories` owns concrete connections. The Server provides only the MemoryLayout KV store and Server Workspace root; `flowcraft_bbh` constructs managed persistence under that root. See [Memory Store](/en/developing/stores/memory) for the complete contract.

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
