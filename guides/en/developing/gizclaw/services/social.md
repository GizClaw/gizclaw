# services/social

`pkgs/gizclaw/services/social` Owns GizClaw’s social graph, including contacts, friend relationships, and friend groups. Each subpackage is responsible for a clear resource boundary.

## Directory structure

```text
services/social/
├── contact/       # Contact resources
├── friend/        # friend requests and friend relationships
└── friendgroup/   # groups, members, messages, and message assets
```

## Subdirectory responsibilities

### contact

Owns peer's contact resources and contact lifecycle. Contact is the address book data maintained by the user, which is not equivalent to the established friend relationship or the underlying giznet peer connection.

### friend

Owns friend-request creation, acceptance, rejection, and friend-relationship reads and deletion. A Friend relationship directly grants both peers access to its system Workspace without creating a generic access role.

Each direct-friend chat lifecycle owns a system Workspace. Creation rollback may immediately delete a Workspace that never became active. Formal friend deletion first atomically removes both relationship rows and stores a minimal retirement intent in one KV `BatchMutate`; only after that commit does it place the Workspace in `PendingDeletion`. Runtime, history, and artifacts are not synchronously deleted on the Social request path. The peer who creates the invite token is the initiator and immutable Workspace owner; the accepting peer receives access without sharing ownership. Admin creation uses its explicit owner. The owner's RuntimeProfile `workflows.system.friend_chatroom` selects the persisted Chatroom Workflow.

### friendgroup

Owns friend groups, members, messages, invites, and message assets. Group membership directly grants access to the group system Workspace.

Each Friend Group lifecycle owns a system Workspace. Creation rollback may immediately delete an unused Workspace. Formal group deletion first atomically removes Group, invite, member, and belongs records and stores a retirement intent in one shared relationship-store transaction. After that commit, it creates a Friend Group data `PendingDeletion` with the message-store and message-asset locators, then places the Workspace in its own `PendingDeletion`. Messages, history, runtime, and artifacts remain physically intact for their owning asynchronous cleaners. A peer-created group belongs to its creator; Admin creation requires an explicit owner. Membership grants data access without changing ownership. The owner's RuntimeProfile `workflows.system.group_chatroom` selects the persisted Chatroom Workflow.

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
    Social --> Assets["Message object store"]
```

Should be placed at `services/social`:

- Domain behaviors for Contact, friend request, friend relationship, group, member and message.
- Validation, storage and cleanup of Social resources.

Shouldn't be placed here:

-Giznet peer connection or signaling contact.
- RuntimeProfile persistence, owner indexes, or generic registration logic. Social only resolves an owner's current profile to select the configured system Workflow before creating domain state.
- Chat Agent, workspace runtime, or generic messaging transport.
- Admin/Peer route registration.

When adding social capabilities, you should first determine whether it belongs to contact, friend, or friend group; only add new sub-packages when new independent resources and life cycles are formed.
