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
    memory_objects_store: flowcraft-memory-objects
```

The references work with both the layered `storage` plus `stores` layout and the supported one-layer `stores` layout. Backend configuration remains on the referenced Store; `agent_host` never contains a directory, DSN, credential, prefix, scope, or inline backend.

| Field | Required capability | Supported backend |
| --- | --- | --- |
| `agent_host.runtime_store` | `objectstore.ObjectStore` | filesystem ObjectStore |
| `agent_host.flowcraft.state_store` | `kv.Store` | Memory or Badger KV |
| `agent_host.flowcraft.history_store` | `logstore.MutableStore` | ClickHouse LogStore; immutable Volc LogStore is rejected |
| `agent_host.flowcraft.memory_objects_store` | `objectstore.ObjectStore` | filesystem ObjectStore |

When `agent_host` is present, it is authoritative. An omitted nested reference disables that optional capability; an unknown name, wrong Store kind, immutable History Store, unknown field, or empty reference fails Server construction instead of falling back. A Flowcraft or Pet Workflow that enables long-term Memory then requires `memory_objects_store` when its Agent is constructed. State and internal Flowcraft History remain optional.

When the whole block is absent, compatibility mode retains the reserved `agenthost` ObjectStore, the `flowcraft-state` Peer KV prefix, the reserved mutable `flowcraft-history` LogStore, and the AgentHost ObjectStore as Flowcraft Memory-object storage.

Changing a reference requires a process restart. GizClaw does not migrate, merge, copy, or delete data when a binding changes. The Store Registry owns every shared backend and closes it once during Server shutdown; Workspace reload and Agent teardown close only per-Agent adapters.

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
