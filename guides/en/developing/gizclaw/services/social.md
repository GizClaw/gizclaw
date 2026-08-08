# services/social

`pkgs/gizclaw/services/social` Owns GizClaw’s social graph, including contacts, friend relationships, and friend groups. Each subpackage is responsible for a clear resource boundary.

## Directory structure

```text
services/social/
├── contact/       # Contact resources
├── friend/        # friend requests and friend relationships
└── friendgroup/   # groups, members, invitations, and Workspace binding
```

## Subdirectory responsibilities

### contact

Owns peer's contact resources and contact lifecycle. Contact is the address book data maintained by the user, which is not equivalent to the established friend relationship or the underlying giznet peer connection.

### friend

Owns friend-request creation, acceptance, rejection, and friend-relationship reads and deletion. A Friend relationship directly grants both peers access to its system Workspace without creating a generic access role.

Peer RPC exposes a Friend relationship's stable identity as `name`, using the other Peer public key already scoped to the authenticated caller; get/info/delete accept that same `name`. Profile presentation is `FriendInfo.display_name`. The deterministic relationship ID remains internal and on Admin/persistence surfaces only.

Each direct-friend chat lifecycle owns an independent system Workspace. The stable `RelationID` identifies only the peer pair. Each transition from no relationship to active creates a new opaque incarnation and derives a new Workspace name from `(RelationID, incarnation)`. A durable creation intent fixes that incarnation, the Workspace owner and name, and the Chatroom Workflow selected from the owner's RuntimeProfile `workflows.system.friend_chatroom`. If the process stops between Workspace creation and reciprocal relationship commit, retry or startup reconciliation reuses the same intent instead of creating another identity. An immutable per-incarnation decision atomically chooses either the reciprocal relationship commit or cancellation, so two servers sharing the relationship store cannot win both transitions. A delete received while that intent is pending records the cancellation decision and removes the never-active Workspace; a delayed creator that loses the decision also repeats that idempotent cleanup instead of allowing startup reconciliation to restore the relationship. Every cleanup compare-and-deletes the pair's current creation intent only when its stored incarnation still matches, so delayed work from one lifecycle cannot remove a re-add lifecycle's recovery intent.

Friend relationship rows carry the exact Peer-visible Workspace name, while their internal binding stores the canonical Workspace ID used by retirement, `PendingDeletion`, runtime, history, and asset cleanup. Formal friend deletion atomically removes both relationship rows and stores a minimal ID-based retirement intent in one KV `BatchMutate`; only after that commit does it enqueue cleanup. A compact retirement receipt preserves the canonical identity and immutable name needed for idempotent completed-delete retries. Re-adding the friend always creates a new Workspace and does not inspect, clear, or reuse the old Workspace's cleanup state. A relationship or binding missing its final-schema identity fields is invalid; there is no legacy identity fallback. The peer who creates the invite token is the initiator and immutable Workspace owner; the accepting peer receives access without sharing ownership. Admin creation uses its explicit owner.

A Friend invite token is an opaque, byte-exact credential. `friend.add` treats only an empty or whitespace-only value as a missing argument; it does not trim or syntax-check any other input, and creates a relationship only for an exact match with a currently active token. Unknown, arbitrarily formatted, whitespace-wrapped, cleared, and expired tokens all return not found, while the caller's own token returns conflict. Store reads, decoding, active-record validation, and expired-record cleanup failures return one redacted internal error. Every rejection leaves Friend relationships and Workspaces unchanged and keeps the underlying Peer connection usable.

### friendgroup

Owns friend groups, members, invites, and the authoritative canonical `friend_group_id -> workspace_id` binding. Admin always addresses a Group by canonical ID. Peer RPC accepts only the current member's local Group `name`, which the service resolves to the canonical ID within that owner/member scope. Different peers may use different names for the same Group and may reuse the same name for their own resources. Group membership directly grants access to the group system Workspace.

Each Friend Group lifecycle owns a system Workspace. Creation rollback may immediately delete an unused Workspace. Formal group deletion first atomically removes Group, invite, member, and belongs records and stores a retirement intent in one shared relationship-store transaction. After that commit, it creates a Friend Group data `PendingDeletion`, then places the Workspace in its own `PendingDeletion`. Workspace History, runtime, and artifacts remain physically intact for their owning asynchronous cleaners. Legacy Friend Group pending-deletion descriptors may still contain retired message-store locators; retries decode but never reopen or clean those stores. A peer-created group belongs to its creator; Admin creation requires an explicit owner. Membership grants data access without changing ownership. The owner's RuntimeProfile `workflows.system.group_chatroom` selects the persisted Chatroom Workflow.

Peer membership objects use `name` as their identity within `friend_group_name`; Admin membership objects retain canonical `id` plus the scoped `name`. Conversation is the only write path for group messages. `server.friend_group.messages.list/get/audio.get` is a read-only Social projection over the bound Workspace History: it loads the group, verifies current membership, resolves the stored Workspace name, and returns stable message `name` values plus `actor_name` attribution and retained History audio. Get/audio selectors use `friend_group_name + history_name`. Friend Group owns no message metadata store, audio store, TTL, or cleanup loop.

The relationship commit and Workspace retirement are two retryable phases. If
phase one fails, both the relationship and Workspace remain usable. If phase
two fails, the retirement intent remains; retrying the same deletion only
finishes `PendingDeletion` for the same Workspace and never restores or
re-deletes the relationship. Relationship invalidation Peer Events are emitted
only after both phases satisfy the success contract.

A revoked active Chatroom does not auto-switch Workspaces. Every new turn checks
the authoritative relationship before forwarding, ASR, model execution, or
history. An invalid turn is not persisted and returns a typed EOS error with
the same `stream_id`. Workspace listing, ordinary get/history, and new explicit
selection continue to deny access according to relationships and
`PendingDeletion`.

## Dependencies and boundaries

```mermaid
flowchart LR
    Surface["Admin / Peer Social surface"] --> Social["services/social"]
    Social --> Workspace["services/ai/workspace"]
    Social --> KV["KV stores"]
    Social --> History["Workspace History"]
```

Should be placed at `services/social`:

- Domain behaviors for Contact, friend request, friend relationship, group, member, and group-to-Workspace resolution.
- Validation, storage, and cleanup of Social relationship resources.

Shouldn't be placed here:

-Giznet peer connection or signaling contact.
- RuntimeProfile persistence, owner indexes, or generic registration logic. Social only resolves an owner's current profile to select the configured system Workflow before creating domain state.
- Chat Agent, workspace runtime, or generic messaging transport.
- Admin/Peer route registration.

When adding social capabilities, you should first determine whether it belongs to contact, friend, or friend group; only add new sub-packages when new independent resources and life cycles are formed.
