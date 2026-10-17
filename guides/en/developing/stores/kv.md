# pkgs/store/kv

`pkgs/store/kv` provides exact-key reads, writes and deletes, plus membership operations and bounded ordered ranges within explicitly addressed collections. String-segment keys define namespaces; the Store does not enumerate database keys.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/kv)

## Core structure and implementation

| Symbol | Function |
| --- | --- |
| `Key` / `Entry` | Express segmentation keys and read results. |
| `Store` | Define exact CRUD, required batch reads, collection operations and atomic mutations. |
| `GetResult` | Own one batch value or per-key error in input order. |
| `Options` | Configure store behaviors such as key separator. |
| `Memory` / `NewMemory` | In-process ordered store. |
| `Badger` / `NewBadger` | Badger-backed persistent implementation. |
| `SQL` / `NewSQLWithDB` | Borrows a SQLite/PostgreSQL pool and maps the logical prefix to a physical table. |
| `Redis` / `NewRedisWithClient` | Borrows a single-node Redis client and preserves the full ordered and atomic Store contract. |
| `TiKV` / `NewTiKVWithClient` | Borrow a TxnKV client for cross-region atomic mutations, collections, and optional regional stale reads. |
| `WithStrongReads` | Read committed authority and fences while retaining prefixes and the shared atomic root. |
| `Prefixed` | Add a fixed key namespace to the existing Store. |

## Ownership Boundary

`kv` Only defines the byte payload and hierarchical key semantics, and does not explain the field type of the payload. Serialization, resource validation, secondary index, and cross-record consistency are the responsibility of the domain service using it. Callers should use stable prefixes to isolate data and cannot rely on the internal key layout of other fields.

## Batch reads

Every Store implements `BatchGet(ctx, keys) []GetResult`; no optional interface or caller fallback is required. Results preserve input order and duplicates, and each successful byte slice is independently owned. Missing, expired and collection keys return `ErrNotFound`. Single-key `Get` retains `ErrWrongType`. Context cancellation and storage errors are reported per key; one failure does not require discarding independent successes.

Redis executes one native `MGET` on the selected reader. Missing and non-string keys are absent; empty and binary strings remain values. TiKV uses one snapshot and one SDK BatchGet call for all keys; the SDK owns Region routing, internal request splitting and concurrency. No external eight-key batching is applied. Memory, Badger and SQL implement batches with at most eight concurrent point reads; queued work stops on cancellation. Prefix and strong-read views forward the required operation with their existing namespace and authority policy.

There is no cross-backend atomic snapshot guarantee or snapshot shared with later calls. TiKV returns a batch-wide SDK error for every requested key when that call fails.

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

For SQLite/PostgreSQL, only change the logical declaration to `storage: database`; `prefix: peers` then also names the backend table. That name cannot overlap another KV prefix or Metrics table on the connector, and closing the logical Store leaves the shared database pool open.

Redis keyvalue Stores require non-empty, clean, pairwise non-overlapping prefixes on each physical connector. The adapter addresses exact keys and collections, uses absolute deadlines, and implements batches, conditional create, and compare-and-mutate atomically on one Redis node. A zero deadline removes an existing expiration. Redis Cluster and multi-endpoint sharding are unsupported because arbitrary-key atomicity is part of the Store contract.

Peer records and routes share the Store named by `services.peer.store`. PeerRun state and each Server's Admin Peer list use local business SQL tables without enumerating central Redis.

### TiKV

`storage.kind: tikv` discovers the cluster using a `pd_addresses` list of `host:port` addresses and uses Transactional KV. Logical `keyvalue` stores require clean, non-overlapping prefixes and share one cross-region atomic root per connector. All writes, mutation conditions, and collection counters use fresh leader transactions. Mutation guards also become write keys to prevent snapshot-isolation write skew. Only definitive write conflicts are retried, with a finite limit; uncertain commits are returned as errors.

```yaml
storage:
  shared:
    kind: tikv
    pd_addresses: [pd-1.example.com:2379, pd-2.example.com:2379, pd-3.example.com:2379]
    tls_ca_file: /run/secrets/tikv-ca.pem
    tls_cert_file: /run/secrets/tikv-client.pem
    tls_key_file: /run/secrets/tikv-client-key.pem
    stale_read_delay: 5s
    read_labels:
      region: local
stores:
  peers:
    kind: keyvalue
    storage: shared
    prefix: peers
services:
  peer:
    store: peers
```

Omit both `stale_read_delay` and `read_labels` for strong reads throughout. When enabled, both are required; the delay must be an integral number of milliseconds from 1ms to 24h. Ordinary queries subtract the delay from a PD timestamp and enable Stale Read. SDK labels express a replica preference: a healthy configured topology prefers the full regional Learner, while slow or unavailable replicas may cause selection of another replica or the central Leader at the same historical timestamp. The delay selects a historical snapshot; it does not guarantee replica availability. Every query still contacts PD. Unavailable or GC-expired snapshots return errors without selecting an older timestamp. Deployment owns replica placement, networking, and a TiKV GC retention window covering the configured delay.

The TLS CA and optional client certificate/key apply to PD, safe-point etcd, and TiKV RPC without mutating global client-go configuration. Storage closes the physical client; logical stores and strong views borrow it.

Entry deadlines, records, and expiry index entries are maintained in one transaction; a zero deadline removes the old expiry index. Reads immediately treat expired values as absent. Store Registry owns one cancellable maintenance worker per TiKV root. It scans only the internal deadline index in batches of at most 128, with at most 32 batches and a 15-second budget per run. It waits one minute after draining the index, or one second while a backlog remains. Each deletion rechecks the record deadline in a fresh transaction, with write-conflict retries preserving concurrent refreshes. `Stores.Close` cancels and joins the worker before physical Storage is closed. Direct `NewTiKVWithClient` borrowers can invoke `PurgeExpired` for the same bounded maintenance; this does not advance the cluster MVCC GC safe point.

### Offline Redis to TiKV migration {#redis-tikv-migration}

`gizclaw kv-migrate <dir> --source <Redis connector> --target <TiKV connector>` reads the workspace `config.yaml` and opens only those two connections. It does not start a Server, generate identities, edit configuration or open unrelated SQL/provider connections. For `redis.readwrite`, it opens only `write_url` and requires Redis `ROLE` to be primary and `INFO cluster` to report `cluster_enabled:0`. Native Redis Cluster is rejected. Redis 7 or newer is required for `PEXPIRETIME`. Imports and verification use strong TiKV reads regardless of regional stale-read settings.

A separate migration directory can declare only the two connectors and selected logical Stores; use absolute TLS file paths. Alternatively add TiKV temporarily to the original configuration. Preserve logical Store names, prefixes, Server identities and endpoints. `--stores peers,social` selects logical KV Stores; the default selects every `keyvalue` Store bound to source or target. Prefixes must be canonical, nonempty and nonoverlapping. Other namespaces are excluded.

1. Back up Redis and configuration. Stop every source/destination writer, including Server deletion and cleanup workers, throughout the full copy and verification window. For large datasets, this window is data-dependent and is not guaranteed to be short.
2. Run `--dry-run` and resolve unsupported types, sizes and destination conflicts.
3. Import. An interrupted import can be rerun with the same configuration: each rerun scans anew, verifies completed identical records and resumes incomplete collections idempotently.
4. Run `--verify-only`. Both source→target and target→source are checked. Missing or different data, types or deadlines, incomplete collections and extra live destination keys cause nonzero exit.
5. After verification succeeds, change selected `stores.*.storage` bindings to TiKV, retaining prefixes and service bindings. Have deployment confirm Learners have caught up and ordinary historical snapshots are later than the final imported commit; allow at least the configured `stale_read_delay` before restoring service. Control-plane reads remain strong. Retain the separate migration configuration for later verification. Remove unused Redis connectors from the final runtime configuration so Server startup no longer connects to Redis; retain logical prefixes.

```sh
gizclaw kv-migrate /srv/gizclaw --source redis-old --target tikv --dry-run
gizclaw kv-migrate /srv/gizclaw --source redis-old --target tikv
gizclaw kv-migrate /srv/gizclaw --source redis-old --target tikv --verify-only
```

Supported types are binary strings (including empty values), absolute millisecond string expirations, persistent Sets and all-zero-score Sorted Sets used as lexicographic indexes. Expired candidates do not become permanent records. Hashes, Lists, Streams, expiring collections, nonzero scores and oversized data fail without skipping or truncation. Strings default to 1 MiB; `--max-value-bytes` accepts 1 byte–4 MiB. Encoded record/member keys are limited to 4096 bytes.

`--scan-count` is a Redis COUNT hint (default 128, maximum 1024); `--workers` bounds concurrent keys per page (default 8, maximum 32). Compact Redis encodings may return an entire collection. TiKV member writes are split into transactions of at most 128 members; the importer does not accumulate the database in memory. Claims protect incomplete collections: ordinary KV access returns `ErrMigrationInProgress` until all members have been imported and verified. This is not an atomic namespace-wide cutover; target writers must remain stopped.

Different live TiKV records are never overwritten and source Redis data is never changed or deleted. SDK diagnostics go to stderr and stdout contains only the JSON report. Reports include scan visits, actual imports, expected dry-run imports and verification counts. SCAN may repeat keys, so visit counts are not a unique database key count. Errors during migration execution emit partial reports and exit nonzero; a failed destination must not be used for cutover.

While writers remain stopped, restoring the Redis configuration provides rollback. After TiKV writes resume, Redis lacks those later writes and cannot be selected directly for rollback. The tool provides no reverse synchronization or online dual writing. Only selected KV namespaces are migrated, excluding business SQL, History, objects, logs and unrelated Redis data.

### Authority and control-plane reads

Peer records, assignment lookup/resolve/refresh, Peer deletion fences/tasks, API key lookups, authentication, and revocation, and Server-composed Social membership/Peer retirement snapshots use strong reads. Redis read/write stores read the writer; TiKV bypasses regional stale reads and opens a fresh leader snapshot. Other Servers can immediately resolve a successful first assignment, and replica lag cannot make an old `expected_version` acceptable. Route resolve/assign also checks the Peer deletion fence and rejects pending deletion, permanent tombstones, and inactive Peers.

`WithStrongReads` does not create one snapshot across separate operations or cancel work that already started. Cross-record invariants still require atomic mutation conditions. Consumers that accept historical results may use ordinary views; authority revocation and deletion completion checks require strong reads.

PostgreSQL mutations acquire transaction advisory locks scoped to the physical table and encoded keys, sorted by lock ID, including absent guards. Ordinary writes, deletes, and conditional mutations share this protocol without a table-wide write lock. Independent keys can progress concurrently; same-key and multi-key atomic operations remain coordinated. Hash collisions only add contention. PostgreSQL mutations clean up expired keys involved in that mutation; reads still treat expired values as absent and attempt key-scoped physical cleanup under the same advisory lock. Cleanup rechecks expiry after locking, preserves concurrent refreshes, and remains best effort on cancellation or database errors. SQLite retains its existing transaction and expiration cleanup behavior.

## Exact collections and atomic index updates

`AddMembers`, `RemoveMembers`, `HasMember`, and `ListMembers` address one complete collection key. Members are opaque strings, independent of the key separator. Adds and removals are idempotent; missing collections return no members, and ordering is unspecified. `ListMembers` reads the entire addressed collection, so callers must scope collections to business resources and avoid global collections. Use `HasMember` for membership checks instead of listing.

`ApplyMutation` atomically checks `Conditions`, writes records, deletes keys, adds members, and removes members in that order. A nil condition `Expected` requires absence; a non-nil value requires an exact record-byte match. Failed conditions return false without writes. Record and collection keys within a mutation must be distinct. Collections have no expiration, and deleting their final member removes the collection.

Redis uses native Sets and Lua. Badger uses distinct physical record/member namespaces and transactions, with exact member-key lookups. SQL uses a record type column and a separate member table whose primary key is `(encoded_key, member)`; transactions coordinate record and collection keys. `Prefixed` scopes records, conditions, and collection keys while leaving member IDs unchanged.

`SetMembers.MaxMembers` on `AddMembers` or `AddOrderedMembers` can bound the collection after that addition; zero means unlimited. An addition that introduces distinct members beyond the limit returns `ErrMemberLimit` without any writes from the mutation. Repeated members remain idempotent, and removals release capacity. Capacity checks and record/member writes share one Lua execution, transaction, or memory lock, preventing concurrent clients from exceeding the limit. Each addition group is checked separately; later removals do not reserve capacity for earlier additions.

## Ordered collection ranges

`RangeOrderedMembers` reads members in ascending byte order within one complete collection key. `After` and `Before` are optional exclusive bounds, and `Limit` must be positive. Members remain opaque strings; the business layer encodes time indexes with fixed-width timestamps and resource IDs. This operation does not interpret key prefixes or enumerate other collections.

`ApplyMutation.AddOrderedMembers` and `RemoveOrderedMembers` atomically maintain ordered indexes with business records. Ordered collections, ordinary Sets, and byte values are distinct types; mixing them returns `ErrWrongType`. Redis uses a Sorted Set with every score equal to zero and evaluates the lexicographic bounds and LIMIT server-side. Badger seeks a bounded range of member keys. SQL applies range predicates, ORDER BY, and LIMIT to the `(encoded_key, member)` index. Memory sorts within the addressed in-process collection. Callers must still control collection size through business scope or partitioning.

`kv.WithPrimaryReads(ctx)` explicitly binds point, Set and ordered-set reads in an operation and its call chain to the write authority (Redis writer or strong TiKV reads). It works through prefixed views, opens no connections and does not change unrelated requests. PendingDeletion management and cleanup use it for ownership, task and absence verification. Ordinary reads retain their configured reader and never fall back automatically. Primary reads do not form a snapshot; final mutations still require atomic conditions.
