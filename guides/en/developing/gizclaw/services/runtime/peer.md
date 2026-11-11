# Peer

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peer)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer)

`peer` Owns server-side persistent Peer resources and implements Peer CRUD, verification, indexing and connected-peer bootstrap required for Admin HTTP and Peer HTTP.

Peer KV mutations derive CAS conditions and identifier indexes from the persisted snapshot without SQL projections. SQL owns the RegistrationToken Firmware binding, exposed through `RegistrationFirmware` on reads. Partial profile updates, device identifiers refresh, and approve/block operations neither compare that projection as the old KV value nor write it back into KV. Mutations resolve their return projection before committing, so a failed SQL lookup cannot report an already committed write as failed. Atomic CAS still rejects stale writes from another Server, with deletion fences and index changes in the same transaction.

Public key is Peer identity and should not be mixed with database ID, connection ID or Edge assignment. WebRTC connection lifecycle belongs to `giznet` and root `PeerManager`, and does not belong to this package.

A Peer maintains at most 10 distinct SN/IMEI indexes: a non-empty SN counts as one, and complete IMEIs are deduplicated by `(TAC, serial)`. Duplicate or incomplete IMEIs add no indexes. Each SN or IMEI reverse Set associates at most 10 distinct Peer public keys. Full saves and device-field refreshes enforce the limit. Collection capacity is checked in the same atomic mutation as the source record and index changes. Rejection preserves the record and every old/new index; duplicate membership uses no extra slot, and removing membership releases capacity. Admin refresh returns `409 DEVICE_IDENTIFIER_LIMIT_REACHED` or `409 DEVICE_IDENTIFIER_INDEX_FULL`. Existing over-limit collections are not pruned automatically; idempotent non-growing additions and removals remain allowed.

Peer deletion atomically creates or reuses a `kind=peer` PendingDeletion identity fence. Admin get/list and repeated deletion remain available for diagnostics. Admission, profile and fence reads follow the configured replica policy, so a successful stale read may still observe active state until its snapshot advances; once visible, the fence rejects reconnects and business access with `PEER_PENDING_DELETION`. Atomic write conditions reject mutations crossing a committed fence. Local connections are quiesced, and a terminal cleanup failure retains the fence.

The production handler validates the fixed home before accessing regional SQL or providers. Task discovery and domain reads start with the [configured reader](../../../stores/kv); a claim mismatch, failed lookup or inconsistent retirement proof permits a bounded primary refresh. Foreign workers leave shared work for the home Server. Unknown or invalid routes retain the task and identity fence, and local SQL absence never proves another region completed cleanup.

Social adapters first persist domain admission fences and prove their inventories include preceding writes. Contacts fence and capture within one SQL transaction; Friend and Group admissions compare permanent Peer fences in their shared atomic roots. Existing plans must cover the prepared inventory or retain the task as `social_snapshot_incomplete`. Captured memberships retain their creation identity through retries, and known missing children require update-time absence validation. Read-only recovery probes do not create fences.

Workspace preparation similarly commits a permanent owner admission fence and the ordinary Workspace inventory in one SQL transaction. Every owned Workspace insertion checks that fence, including another service instance that still observes an active Peer. Existing plans must cover the fenced SQL inventory or retain the task as `workspace_snapshot_incomplete`.

Admin pages skip local directory entries absent from the historical Peer reader and advance by the last scanned local key, without upgrading the query. SQL Contact and Workspace creation may refresh a missing Peer input once; ordinary Workspace reads use a separate availability callback and retain historical absence.

Retirement plans bind the marker fingerprint, home Server and exact unprojected KV generation. The handler validates the generation before domain side effects and compares the exact record bytes again in the atomic tombstone/task mutation. Removing the SQL RegistrationFirmware display projection does not change that generation. Local Contacts and user Workspaces are retired through their domain adapters. Child Workspace memory, History, runtime, icons, objects and graph state must complete before Peer-scoped memory and the regional RuntimeProfile owner binding are removed. Global catalogs, registration tokens, foreign resources, logs and metrics remain. PeerRun remains the local diagnostic directory.

Friend and Friend Group system Workspaces can have regional SQL replicas. The shared Social records do not contain a complete replica-completion ledger, so Peer plans selecting a Friend or owned Friend Group Workspace remain fenced with `social_replica_scope_unknown`. A Friend Group KV task proves its shared control-plane retirement, not physical removal of every regional Workspace replica.

Version 2 plans are accepted only when they contain no regional children, the full retained Peer matches (allowing only a projected firmware value absent from raw KV), and a fresh home snapshot finds no Contacts, Social relationships or Workspaces omitted from that plan. Adoption conditionally writes a home-bound raw-generation plan. Ambiguous scopes, unsupported versions, missing records and actual generation changes retain the task for evidence-backed repair.

Successful finalization atomically removes exact secondary indexes, plan, marker, locator, task and task indexes and writes `{"version":1,"state":"deleted"}` at `by-pubkey/<public-key>`. Admin get/list derive `{public_key,status=deleted}`; other entry points return `PEER_DELETED`, permanently preventing registration with that key. Fixed routes retain ownership evidence, API Key retired markers retain concurrent-creation fences, and unrelated data survives. On startup and each background scan, the home Server automatically recovers only finalize-stage legacy Firmware projection failures that pass the raw-generation and empty-home evidence checks. Recovery shares existing worker bounds and conditionally requeues the unchanged failure. The adopted plan cannot automatically reset its retry budget again. Other terminal failures remain retained. See optional [online recovery](../../../../using/cli#pendingdeletion-recovery).

Edge bootstrap avoids rewriting an already active Edge. When Servers initialize the same Edge concurrently, CAS conflicts trigger at most eight attempts with a fresh record each time. Role and status are merged into the current record, preserving concurrent metadata and identifier indexes while retaining deletion fences.

`blocked` is durable state that Admin approve can restore to active.
`EnsureConnectedPeer` and `EnsureConnectedPeerGuarded` return `ErrPeerBlocked`
for existing blocked records, including a concurrent create winner. They do not
interpret them as missing or recreate them. After Admin block commits, all local
connection generations are detached and the old generation's atomic retiring
flag is set under the Manager lock. Callbacks and transport close run outside locks. Blocking does not create a local
runtime-directory entry for a remote/offline Peer and does not depend on that
directory being available. A failed durable write does not disconnect a healthy Peer. `EnsureAvailable` retains deletion-fence
semantics so Admin can still read, edit, and approve blocked records. Connection
and service enforcement belong to [Management](../../peer/manager) and
[Security Policy](../../server/security-policy).

`Server.CheckActiveRole` reads current Peer KV authority and deletion fences without the SQL firmware display projection. It requires active status and an exact role; KV errors, pending deletion and permanent tombstones deny access. It retains the caller context and caches no decision. Admin/Edge transport admission uses this predicate, while full resource reads through `LoadPeer` retain firmware projection.

Membership in a foreign-owned group is removed through its shared membership adapter; the foreign group and its Workspace remain outside the retiring Peer scope.
