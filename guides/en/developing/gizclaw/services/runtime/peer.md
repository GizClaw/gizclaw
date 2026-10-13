# Peer

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer)

`peer` Owns server-side persistent Peer resources and implements Peer CRUD, verification, indexing and connected-peer bootstrap required for Admin HTTP and Peer HTTP.

Peer KV mutations derive CAS conditions and identifier indexes from the persisted snapshot without SQL projections. SQL owns the RegistrationToken Firmware binding, exposed through `RegistrationFirmware` on reads. Partial profile updates, device identifiers refresh, and approve/block operations neither compare that projection as the old KV value nor write it back into KV. Mutations resolve their return projection before committing, so a failed SQL lookup cannot report an already committed write as failed. Atomic CAS still rejects stale writes from another Server, with deletion fences and index changes in the same transaction.

## Core structure and main function

| Structure or function | Function |
| --- | --- |
| `Server` | Combines Peer store, online `PeerManager` and HTTP service dependencies. |
| `PeerManager` | Query online Peer connection/runtime, does not have persistent records. |
| `PeerAdminService` | Define the Peer operations required by the Admin surface. |
| `PeerHTTPService` | Define the Peer operations required for Peer-facing surface. |
| `Server.EnsureConnectedPeer` / `EnsureConnectedPeerGuarded` | Create a default active Peer for the authenticated public key; the guarded form revalidates connection lifecycle state under the per-record lock before reading or creating it. |
| `Server.LoadPeer` / `SavePeer` | Press public key to read or save the complete Peer. |
| `Server.BootstrapEdgeNodes` | Synchronize the Edge Node identity in the configuration as a Peer resource. |
| `Server.DeleteSelf` | Atomically create or reuse a durable pending-deletion handoff for the authenticated Peer. |

Public key is Peer identity and should not be mixed with database ID, connection ID or Edge assignment. WebRTC connection lifecycle belongs to `giznet` and root `PeerManager`, and does not belong to this package.

A Peer maintains at most 10 distinct SN/IMEI indexes: a non-empty SN counts as one, and complete IMEIs are deduplicated by `(TAC, serial)`. Duplicate or incomplete IMEIs add no indexes. Each SN or IMEI reverse Set associates at most 10 distinct Peer public keys. Full saves and device-field refreshes enforce the limit. Collection capacity is checked in the same atomic mutation as the source record and index changes. Rejection preserves the record and every old/new index; duplicate membership uses no extra slot, and removing membership releases capacity. Admin refresh returns `409 DEVICE_IDENTIFIER_LIMIT_REACHED` or `409 DEVICE_IDENTIFIER_INDEX_FULL`. Existing over-limit collections are not pruned automatically; idempotent non-growing additions and removals remain allowed.

Peer deletion creates or reuses one `kind=peer` PendingDeletion in the Peer KV and immediately turns the public key into a cross-process identity fence. While the marker exists, Admin Peer get/list and the same delete remain available for diagnostics and idempotent retry. Reconnect, API key HTTP, WebRTC, RPC/streams, Edge bootstrap, business reads, and mutations return `PEER_PENDING_DELETION`. The online connection is quiesced, and a terminal cleanup failure does not lift the fence.

The deletion handler validates the fixed home Server from the central Redis writer before accessing regional SQL or providers. Foreign workers leave the shared task unclaimed; operator retry returns `peer_home_mismatch`. Unknown or invalid assignments keep the task and identity fence. Local SQL absence never proves another region completed cleanup.

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
