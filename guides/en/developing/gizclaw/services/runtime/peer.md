# Peer

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peer)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer)

`peer` Owns server-side persistent Peer resources and implements Peer CRUD, verification, indexing and connected-peer bootstrap required for Admin HTTP and Peer HTTP.

Peer KV mutations derive CAS conditions and identifier indexes from the persisted snapshot without SQL projections. SQL owns the RegistrationToken Firmware binding, exposed through `RegistrationFirmware` on reads. Partial profile updates, device identifiers refresh, and approve/block operations neither compare that projection as the old KV value nor write it back into KV. Mutations resolve their return projection before committing, so a failed SQL lookup cannot report an already committed write as failed. Atomic CAS still rejects stale writes from another Server, with deletion fences and index changes in the same transaction.

Public key is Peer identity and should not be mixed with database ID, connection ID or Edge assignment. WebRTC connection lifecycle belongs to `giznet` and root `PeerManager`, and does not belong to this package.

A Peer maintains at most 10 distinct SN/IMEI indexes: a non-empty SN counts as one, and complete IMEIs are deduplicated by `(TAC, serial)`. Duplicate or incomplete IMEIs add no indexes. Each SN or IMEI reverse Set associates at most 10 distinct Peer public keys. Full saves and device-field refreshes enforce the limit. Collection capacity is checked in the same atomic mutation as the source record and index changes. Rejection preserves the record and every old/new index; duplicate membership uses no extra slot, and removing membership releases capacity. Admin refresh returns `409 DEVICE_IDENTIFIER_LIMIT_REACHED` or `409 DEVICE_IDENTIFIER_INDEX_FULL`. Existing over-limit collections are not pruned automatically; idempotent non-growing additions and removals remain allowed.

A Peer deletion request atomically publishes the permanent `deleted` tombstone and creates or reuses its durable cleanup request. The request retains the original unprojected Peer bytes for cleanup; repeating deletion does not replace the request or reset its progress. Admin get/list expose the tombstone immediately through the local write view, while other Server instances observe it according to the configured replica policy. Once visible, it rejects activation and business access with `PEER_DELETED`. Atomic mutation conditions prevent an old active observation from replacing the tombstone. Local connections are quiesced after the request commits.

The production handler validates the fixed home before accessing regional SQL or providers. Task discovery and domain reads start with the [configured reader](../../../stores/kv); a claim mismatch, failed lookup or inconsistent retirement proof permits a bounded primary refresh. Foreign workers leave shared work for the home Server. Unknown or invalid routes retain the task and identity fence, and local SQL absence never proves another region completed cleanup.

Cleanup starts when the task is discovered and repeats hourly for at least 32 completed executions. The KV task persists its execution count separately from failures; normal deferral does not consume the failure budget. Transient failures also retry hourly with a failure limit of at least 32. Each execution rebuilds the Social and Workspace inventories instead of treating a previous empty or partial snapshot as complete. Unfinished exact lifecycles remain in the durable plan across inventory refreshes. Successful handoffs are checkpointed individually, so later failures neither erase outstanding work nor replay completed lifecycles. Already dispatched child task identities remain tracked across passes, and their apparent absence is checked at the write authority before completion. A successful execution numbered 32 or later may finish only after its known cleanup and child tasks complete; otherwise the request remains. The RuntimeProfile owner binding stays available throughout these passes.

Contacts and ordinary Workspaces fence owner admission and capture their local SQL inventory transactionally. Friend and Group admission mutations check permanent Peer fences in their shared atomic roots. Reading back a locally committed fence does not prove that a replica has exposed every earlier relationship; later executions discover and clean late-visible records. Captured membership identities still protect each individual deletion from removing a different incarnation.

Admin pages skip local directory entries absent from the historical Peer reader and advance by the last scanned local key, without upgrading the query. SQL Contact and Workspace creation may refresh a missing Peer input once; ordinary Workspace reads use a separate availability callback. Read/write-separated KV backends make their own committed local writes visible through that callback; historical absence from other backend instances remains possible.

Retirement plans bind the marker fingerprint, home Server and exact original Peer bytes retained by the deletion request. Each refresh verifies the tombstone and compares the previous plan atomically before replacing its inventory. A genuine Peer replacement or invalid task remains fenced for investigation. SQL RegistrationFirmware display projections do not change the retained identity. Domain adapters retire local Contacts and user Workspaces. Known child Workspace memory, History, runtime, icons, objects and Graph state complete before the final removal of the RuntimeProfile owner binding. Global catalogs, registration tokens, foreign resources, logs and metrics remain. PeerRun remains the local diagnostic directory.

Friend and Friend Group membership and room bindings are authoritative in shared KV. Peer cleanup retires the relationships it discovers and hands their known children to the owning domain adapters; it does not require a global SQL replica-completion ledger before processing them. Regional system Workspace rows are local runtime catalog replicas. The bounded repeated passes do not prove physical removal of every regional SQL replica or discovery of data that remains invisible throughout all executions.

Version 2 plans are accepted only when they contain no regional children, the full retained Peer matches (allowing only a projected firmware value absent from raw KV), and a fresh home snapshot finds no Contacts, Social relationships or Workspaces omitted from that plan. Adoption conditionally writes a home-bound raw-generation plan. Ambiguous scopes, unsupported versions, missing records and actual generation changes retain the task for evidence-backed repair.

After the required executions and a successful final cleanup pass, guarded finalization atomically consumes the exact secondary indexes, plan, marker, locator, task and task indexes while preserving `{"version":1,"state":"deleted"}` at `by-pubkey/<public-key>`. Admin get/list derive `{public_key,status=deleted}`; other entry points return `PEER_DELETED`, permanently preventing registration with that key. Fixed routes retain ownership evidence, API Key retired markers retain concurrent-creation fences, and unrelated data survives. On startup and each background scan, the home Server automatically recovers only finalize-stage legacy Firmware projection failures that pass the raw-generation and empty-home evidence checks. Recovery shares existing worker bounds and conditionally requeues the unchanged failure. The adopted plan cannot automatically reset its retry budget again. Other terminal failures remain retained. See optional [online recovery](../../../../using/cli#pendingdeletion-recovery).

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
