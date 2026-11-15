# pkgs/store/kv

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/store/kv)

`pkgs/store/kv` provides exact-key reads, writes and deletes, plus membership operations and bounded ordered ranges within explicitly addressed collections. String-segment keys define namespaces; the Store does not enumerate database keys.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/kv)

## Ownership Boundary

`kv` Only defines the byte payload and hierarchical key semantics, and does not explain the field type of the payload. Serialization, resource validation, secondary index, and cross-record consistency are the responsibility of the domain service using it. Callers should use stable prefixes to isolate data and cannot rely on the internal key layout of other fields.

## Batch reads

Every Store implements `BatchGet(ctx, keys) []GetResult`; no optional interface or caller fallback is required. Results preserve input order and duplicates, and each successful byte slice is independently owned. Missing, expired and collection keys return `ErrNotFound`. Single-key `Get` retains `ErrWrongType`. Context cancellation and storage errors are reported per key; one failure does not require discarding independent successes.

Redis executes one native `MGET` on the selected reader. Missing and non-string keys are absent; empty and binary strings remain values. TiKV uses one snapshot and one SDK BatchGet call with unique physical record keys, then maps results back to every original input position; the SDK owns Region routing, internal request splitting and concurrency. Transactional mutation prefetch also deduplicates physical keys without changing conditions or guard writebacks, preventing duplicate cleanup of the same residual lock. No external eight-key batching is applied. Memory, Badger and SQL implement batches with at most eight concurrent point reads; queued work stops on cancellation. Prefix and strong-read views forward the required operation with their existing namespace and authority policy.

There is no cross-backend atomic snapshot guarantee or snapshot shared with later calls. Read/write-separated backends merge committed local postimages into ordinary results; a backend read failure is propagated for keys not satisfied by those postimages. Cancellation remains an error for every affected key.

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

### Local write visibility with read/write separation

`redis.readwrite` and TiKV regional readers retain committed local writes in backend-owned memory. This provides read-your-writes within that backend instance without changing `kv.Store`, business values, prefixes, or atomic transaction boundaries. Records, batch and conditional writes, deletions, absolute expirations, and ordinary/ordered collection changes participate. Failed conditions and uncertain commits do not publish attempted values. Explicit primary reads bypass the local postimages. Other instances' writes may remain stale, and process restart starts a new local consistency scope.

Read/write-separated Redis requires Redis 7 or newer for exact absolute expiration metadata; unsupported expiration commands are rejected before business mutations run. Redis captures typed postimages in the writer's successful atomic operation and uses replication identity/offset metadata to prove reader coverage. Deployment must allow `INFO replication` for progress tracking. Without trustworthy progress, pending postimages remain retained. Covered postimages are reclaimed while a compact required replication position is retained; subsequent lagging, replaced or unverifiable readers return errors for uncovered keys rather than returning state older than retired local writes. Reader proof and data belong to the same query attempt; there is no automatic primary fallback. Ordinary reads opportunistically reclaim all covered pending postimages; write-only workloads retain them until an ordinary read runs. No reader network request is added to the write path and no hidden worker is started. Ordinary single-endpoint Redis does not enable this layer.

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

Configure `read_labels` alone to enable regional reads; omit it or use an empty map for strong reads throughout. Ordinary data reads go directly to label-matching Learners using native TiKV replica-read synchronization. Snapshots use the cached PD timestamp without subtracting a delay; only a missing cache requires a fresh TSO, and the data read still targets a matching Learner. The application does not query or select safe-ts. Native synchronization may require ReadIndex and coordination with the Leader. A strict RPC guard rejects Leader and nonmatching peers for ordinary data reads; failures propagate instead of switching the data read to another authority. There is no configured maximum staleness or fixed default snapshot delay. The deprecated `stale_read_delay` YAML option and Go `TiKVConfig.StaleReadDelay` field are accepted but ignored; YAML duration text is not parsed. Writes, mutation conditions and explicit primary views remain authoritative and obtain fresh timestamps. Deployment owns replica placement, networking and MVCC GC retention.

Regional TiKV readers merge this backend instance's committed physical postimages according to the actual read timestamp. This covers records, collection members, resets, deletions and absolute expiration without changing persistent payload formats. Concurrent transactions publish by commit order, and each read pins its relevant postimages before timestamp selection. Postimages can be reclaimed only when timestamp coverage preserves local write visibility; cached PD time alone does not prove regional replica coverage. Explicit primary reads bypass this layer. Internal expiry-index discovery retains its existing bounded maintenance path. Strong-only TiKV instances do not allocate the local write layer. Read-your-writes is scoped to one backend instance; it is not guaranteed across instances or process restarts.

The TLS CA and optional client certificate/key apply to PD, safe-point etcd, and TiKV RPC without mutating global client-go configuration. Storage closes the physical client; logical stores and strong views borrow it.

Entry deadlines, records, and expiry index entries are maintained in one transaction; a zero deadline removes the old expiry index. Reads immediately treat expired values as absent. Store Registry owns one cancellable maintenance worker per TiKV root. It scans only the internal deadline index in batches of at most 128, with at most 32 batches and a 15-second budget per run. It waits one minute after draining the index, or one second while a backlog remains. Each deletion rechecks the record deadline in a fresh transaction, with write-conflict retries preserving concurrent refreshes. If that transaction proves a historical candidate was already consumed, the uncommitted batch is discarded and discovery is repeated once on primary; the returned count includes only committed index removals. `Stores.Close` cancels and joins the worker before physical Storage is closed. Direct `NewTiKVWithClient` borrowers can invoke `PurgeExpired` for the same bounded maintenance; this does not advance the cluster MVCC GC safe point.

### Offline Redis to TiKV migration {#redis-tikv-migration}

`gizclaw kv-migrate <dir> --source <Redis connector> --target <TiKV connector>` reads the workspace `config.yaml` and opens only those two connections. It does not start a Server, generate identities, edit configuration or open unrelated SQL/provider connections. For `redis.readwrite`, it opens only `write_url` and requires Redis `ROLE` to be primary and `INFO cluster` to report `cluster_enabled:0`. Native Redis Cluster is rejected. Redis 7 or newer is required for `PEXPIRETIME`. Imports and verification use strong TiKV reads regardless of regional read settings.

A separate migration directory can declare only the two connectors and selected logical Stores; use absolute TLS file paths. Alternatively add TiKV temporarily to the original configuration. Preserve logical Store names, prefixes, Server identities and endpoints. `--stores peers,social` selects logical KV Stores; the default selects every `keyvalue` Store bound to source or target. Prefixes must be canonical, nonempty and nonoverlapping. Other namespaces are excluded.

1. Back up Redis and configuration. Stop every source/destination writer, including Server deletion and cleanup workers, throughout the full copy and verification window. For large datasets, this window is data-dependent and is not guaranteed to be short.
2. Run `--dry-run` and resolve unsupported types, sizes and destination conflicts.
3. Import. An interrupted import can be rerun with the same configuration: each rerun scans anew, verifies completed identical records and resumes incomplete collections idempotently.
4. Run `--verify-only`. Both source→target and target→source are checked. Missing or different data, types or deadlines, incomplete collections and extra live destination keys cause nonzero exit.
5. After verification succeeds, change selected `stores.*.storage` bindings to TiKV, retaining prefixes and service bindings. Before restoring service, have deployment confirm that matching Learners can serve native synchronized reads at timestamps later than the final imported commit. Control-plane queries follow the same configured read policy. Retain the separate migration configuration for later verification. Remove unused Redis connectors from the final runtime configuration so Server startup no longer connects to Redis; retain logical prefixes.

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

Peer records and profiles, assignments, deletion fences and tasks, API key authentication, Social relationships, synchronization checkpoints and infrastructure feeds all follow the configured read policy. With regional TiKV reads enabled, successful ordinary reads may retain an old assignment, active Peer, credential or membership written by another backend instance until the read timestamp advances. This instance's committed local writes are visible through its internal write overlay. Revocation and deletion visibility follows that read timestamp; a successful read does not itself trigger a primary lookup.

Writes and their conditions execute on the write authority. A definite conditional conflict, failed identity lookup or observation incompatible with a known successful claim can trigger a bounded primary refresh. Retries preserve explicit versions, fixed ownership, lease tokens and lifecycle identities; uncertain commits are not replayed. Primary fallback applies only to the affected operation. `WithStrongReads` remains available for explicit authority views and offline verification, and does not create a snapshot across separate calls.

PostgreSQL mutations acquire transaction advisory locks scoped to the physical table and encoded keys, sorted by lock ID, including absent guards. Ordinary writes, deletes, and conditional mutations share this protocol without a table-wide write lock. Independent keys can progress concurrently; same-key and multi-key atomic operations remain coordinated. Hash collisions only add contention. PostgreSQL mutations clean up expired keys involved in that mutation; reads still treat expired values as absent and attempt key-scoped physical cleanup under the same advisory lock. Cleanup rechecks expiry after locking, preserves concurrent refreshes, and remains best effort on cancellation or database errors. SQLite retains its existing transaction and expiration cleanup behavior.

## Exact collections and atomic index updates

`AddMembers`, `RemoveMembers`, `HasMember`, and `ListMembers` address one complete collection key. Members are opaque strings, independent of the key separator. Adds and removals are idempotent; missing collections return no members, and ordering is unspecified. `ListMembers` reads the entire addressed collection, so callers must scope collections to business resources and avoid global collections. Use `HasMember` for membership checks instead of listing.

`ApplyMutation` atomically checks `Conditions`, writes records, deletes keys, adds members, and removes members in that order. A nil condition `Expected` requires absence; a non-nil value requires an exact record-byte match. Failed conditions return false without writes. Record and collection keys within a mutation must be distinct. Collections have no expiration, and deleting their final member removes the collection.

Redis uses native Sets and Lua. Badger uses distinct physical record/member namespaces and transactions, with exact member-key lookups. SQL uses a record type column and a separate member table whose primary key is `(encoded_key, member)`; transactions coordinate record and collection keys. `Prefixed` scopes records, conditions, and collection keys while leaving member IDs unchanged.

`SetMembers.MaxMembers` on `AddMembers` or `AddOrderedMembers` can bound the collection after that addition; zero means unlimited. An addition that introduces distinct members beyond the limit returns `ErrMemberLimit` without any writes from the mutation. Repeated members remain idempotent, and removals release capacity. Capacity checks and record/member writes share one Lua execution, transaction, or memory lock, preventing concurrent clients from exceeding the limit. Each addition group is checked separately; later removals do not reserve capacity for earlier additions.

## Ordered collection ranges

`RangeOrderedMembers` reads members in ascending byte order within one complete collection key. `After` and `Before` are optional exclusive bounds, and `Limit` must be positive. Members remain opaque strings; the business layer encodes time indexes with fixed-width timestamps and resource IDs. This operation does not interpret key prefixes or enumerate other collections.

`ApplyMutation.AddOrderedMembers` and `RemoveOrderedMembers` atomically maintain ordered indexes with business records. Ordered collections, ordinary Sets, and byte values are distinct types; mixing them returns `ErrWrongType`. Redis uses a Sorted Set with every score equal to zero and evaluates the lexicographic bounds and LIMIT server-side. Badger seeks a bounded range of member keys. SQL applies range predicates, ORDER BY, and LIMIT to the `(encoded_key, member)` index. Memory sorts within the addressed in-process collection. Callers must still control collection size through business scope or partitioning.

`kv.WithPrimaryReads(ctx)` selects the write authority for bounded update recovery after a rejected condition or an observation inconsistent with a known successful write. A missing target may be refreshed within a mutation such as API Key revocation. Ordinary queries, including authentication negatives and PendingDeletion administration/discovery, retain the configured reader with backend-local write visibility; writes from other backend instances may still produce stale absence or errors. The fallback preserves cancellation, deadlines and prefixes, does not pin unrelated requests, and does not combine reads into one snapshot. Final writes still compare their conditions atomically.
