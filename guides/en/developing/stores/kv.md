# pkgs/store/kv

`pkgs/store/kv` Defines GizClaw’s general ordered key-value abstraction. Key uses string segments to express hierarchical paths, and Store provides get, set, delete, prefix list and ordered traversal capabilities.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/kv)

## Core structure and implementation

| Symbol | Function |
| --- | --- |
| `Key` / `Entry` | Express segmentation keys and read results. |
| `Store` | Define CRUD, prefix listing and iterator contract. |
| `Options` | Configure store behaviors such as key separator. |
| `Memory` / `NewMemory` | In-process ordered store. |
| `Badger` / `NewBadger` | Badger-backed persistent implementation. |
| `SQL` / `NewSQLWithDB` | Borrows a SQLite/PostgreSQL pool and maps the logical prefix to a physical table. |
| `Redis` / `NewRedisWithClient` | Borrows a single-node Redis client and preserves the full ordered and atomic Store contract. |
| `Prefixed` | Add a fixed key namespace to the existing Store. |
| `ListAfter` | Read in pages after the specified key under prefix. |

## Ownership Boundary

`kv` Only defines the byte payload and hierarchical key semantics, and does not explain the field type of the payload. Serialization, resource validation, secondary index, and cross-record consistency are the responsibility of the domain service using it. Callers should use stable prefixes to isolate data and cannot rely on the internal key layout of other fields.

## Server composition

A `storage.kind: badger` entry opens one `*badger.DB`; logical Stores borrow it through `NewBadgerWithDB`. `storage.kind: memory` is only a marker, and every logical keyvalue Store creates an independent `*kv.Memory`. A SQLite/PostgreSQL keyvalue Store declares only one required, single-segment `prefix`. The backend uses it directly as the quoted physical table name, rejects `table`, and does not add the prefix again to encoded keys. The SQL implementation preserves ordered prefixes, native paging, deadlines, batches, conditional create, and compare-and-mutate with database transactions. Expired rows are removed by reads or later successful mutations; there is no background worker. The table stores opaque bytes and never replaces a domain SQL repository automatically. Fixed `services` fields name the logical Store.

```yaml
storage:
  state:
    kind: badger
    dir: data/kv
stores:
  peer-records:
    kind: keyvalue
    storage: state
    prefix: peers
services:
  peer:
    store: peer-records
```

For SQLite/PostgreSQL, only change the logical declaration to `storage: database`; `prefix: peers` then also names the backend table. That name cannot overlap another KV prefix or Metrics/Log table on the connector, and closing the logical Store leaves the shared database pool open.

Redis keyvalue Stores require non-empty, clean, pairwise non-overlapping prefixes on each physical connector. The adapter sorts SCAN results before exposing them, uses absolute deadlines, and implements batches, conditional create, and compare-and-mutate atomically on one Redis node. A zero deadline removes an existing expiration. Redis Cluster and multi-endpoint sharding are unsupported because arbitrary-key atomicity is part of the Store contract.

Peer routes and run state use code-owned prefixes rather than separate operator Store bindings.
