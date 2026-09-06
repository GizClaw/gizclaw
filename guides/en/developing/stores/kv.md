# pkgs/store/kv

`pkgs/store/kv` provides exact-key reads, writes and deletes, plus membership operations and bounded ordered ranges within explicitly addressed collections. String-segment keys define namespaces; the Store does not enumerate database keys.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/kv)

## Core structure and implementation

| Symbol | Function |
| --- | --- |
| `Key` / `Entry` | Express segmentation keys and read results. |
| `Store` | Define exact CRUD, collection operations and atomic mutations. |
| `Options` | Configure store behaviors such as key separator. |
| `Memory` / `NewMemory` | In-process ordered store. |
| `Badger` / `NewBadger` | Badger-backed persistent implementation. |
| `SQL` / `NewSQLWithDB` | Borrows a SQLite/PostgreSQL pool and maps the logical prefix to a physical table. |
| `Redis` / `NewRedisWithClient` | Borrows a single-node Redis client and preserves the full ordered and atomic Store contract. |
| `Prefixed` | Add a fixed key namespace to the existing Store. |

## Ownership Boundary

`kv` Only defines the byte payload and hierarchical key semantics, and does not explain the field type of the payload. Serialization, resource validation, secondary index, and cross-record consistency are the responsibility of the domain service using it. Callers should use stable prefixes to isolate data and cannot rely on the internal key layout of other fields.

## Server composition

A `storage.kind: badger` entry opens one `*badger.DB`; logical Stores borrow it through `NewBadgerWithDB`. `storage.kind: memory` is only a marker, and every logical keyvalue Store creates an independent `*kv.Memory`. A SQLite/PostgreSQL keyvalue Store declares only one required, single-segment `prefix`. The backend uses it directly as the quoted physical table name, rejects `table`, and does not add the prefix again to encoded keys. The SQL implementation preserves deadlines, batches, conditional create, compare-and-mutate and collection operations with database transactions. Expired rows are removed by reads or later successful mutations; there is no background worker. The table stores opaque bytes and never replaces a domain SQL repository automatically. Fixed `services` fields name the logical Store.

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

Redis keyvalue Stores require non-empty, clean, pairwise non-overlapping prefixes on each physical connector. The adapter addresses exact keys and collections, uses absolute deadlines, and implements batches, conditional create, and compare-and-mutate atomically on one Redis node. A zero deadline removes an existing expiration. Redis Cluster and multi-endpoint sharding are unsupported because arbitrary-key atomicity is part of the Store contract.

Peer records and routes share the Store named by `services.peer.store`. PeerRun state and each Server's Admin Peer list use local business SQL tables without enumerating central Redis.

PostgreSQL mutations acquire transaction advisory locks scoped to the physical table and encoded keys, sorted by lock ID, including absent guards. Ordinary writes, deletes, and conditional mutations share this protocol without a table-wide write lock. Independent keys can progress concurrently; same-key and multi-key atomic operations remain coordinated. Hash collisions only add contention. PostgreSQL mutations clean up expired keys involved in that mutation; reads still treat expired values as absent and attempt key-scoped physical cleanup under the same advisory lock. Cleanup rechecks expiry after locking, preserves concurrent refreshes, and remains best effort on cancellation or database errors. SQLite retains its existing transaction and expiration cleanup behavior.

## Exact collections and atomic index updates

`AddMembers`, `RemoveMembers`, `HasMember`, and `ListMembers` address one complete collection key. Members are opaque strings, independent of the key separator. Adds and removals are idempotent; missing collections return no members, and ordering is unspecified. `ListMembers` reads the entire addressed collection, so callers must scope collections to business resources and avoid global collections. Use `HasMember` for membership checks instead of listing.

`ApplyMutation` atomically checks `Conditions`, writes records, deletes keys, adds members, and removes members in that order. A nil condition `Expected` requires absence; a non-nil value requires an exact record-byte match. Failed conditions return false without writes. Record and collection keys within a mutation must be distinct. Collections have no expiration, and deleting their final member removes the collection.

Redis uses native Sets and Lua. Badger uses distinct physical record/member namespaces and transactions, with exact member-key lookups. SQL uses a record type column and a separate member table whose primary key is `(encoded_key, member)`; transactions coordinate record and collection keys. `Prefixed` scopes records, conditions, and collection keys while leaving member IDs unchanged.

## Ordered collection ranges

`RangeOrderedMembers` reads members in ascending byte order within one complete collection key. `After` and `Before` are optional exclusive bounds, and `Limit` must be positive. Members remain opaque strings; the business layer encodes time indexes with fixed-width timestamps and resource IDs. This operation does not interpret key prefixes or enumerate other collections.

`ApplyMutation.AddOrderedMembers` and `RemoveOrderedMembers` atomically maintain ordered indexes with business records. Ordered collections, ordinary Sets, and byte values are distinct types; mixing them returns `ErrWrongType`. Redis uses a Sorted Set with every score equal to zero and evaluates the lexicographic bounds and LIMIT server-side. Badger seeks a bounded range of member keys. SQL applies range predicates, ORDER BY, and LIMIT to the `(encoded_key, member)` index. Memory sorts within the addressed in-process collection. Callers must still control collection size through business scope or partitioning.
