# Server Provided to Client

These methods are implemented by Server and called by a Client/Device through its Peer connection.

The [RPC API Reference](/references/rpc) is the single list of exact method IDs, names, groups, and purposes. This page only explains Server-provided resource projection, call flow, and implementation ownership. See [Server Provided to Edge-node](./server-provided-to-edge-node) for the authorization boundary of Edge-node-only methods.

## RuntimeProfile resource projection

Canonical Workflow, Model, Credential, Voice, and Tool resources are Admin-managed. Peer RPC has no Workflow, Model, Credential, or Tool create/put/delete methods and no `source=runtime|owned` selector.

RuntimeProfile binding aliases are grouped under Collections, but the Peer boundary projects each binding as an immutable `name`. `server.workflow.list` requires a Collection; Workflow, Model, Voice, and Tool get/list requests and responses consistently use names. Responses use `runtime_profile_name`; because RuntimeProfile has no separate Peer alias, its value is the canonical RuntimeProfile ID projected verbatim as the Peer name, alongside the revision. This is the normal Peer projection rule, not a compatibility field. Other canonical IDs, provider configuration, credentials, ownership, and executor routing stay on the Server.

Workspace create requires `collection` and `workflow_name`. The Server resolves that Peer name to the current RuntimeProfile binding and records Collection through an internal Workspace label. Workspace list requires Collection and performs exact filtering, but generic labels are not part of the Peer response. Removing the binding does not hide or delete an existing Workspace; reload/run reports not found until the name resolves again.

## Calling relationship

```mermaid
sequenceDiagram
    participant Client
    participant RPC as Server RPC
    participant Profile as RuntimeProfile snapshot
    participant Service as Domain service
    Client->>RPC: typed request
    RPC->>Profile: resolve Peer names and policy
    RPC->>Service: typed command/query
    Service-->>RPC: result / domain error
    RPC-->>Client: typed response / frames / RPC error
```

The RPC adapter owns payload decoding, framing, lifecycle, and stable error mapping. Domain services own storage, resource validation, authorization, and execution.

The authenticated Peer connection is the root API-key management surface. `server.api_key.create` (method 96), `server.api_key.list` (method 97), and `server.api_key.revoke` (method 98) always derive the owner from the active Client connection and verify its durable RuntimeProfile owner binding. Create and cursor-paginated list return complete recoverable API keys; revoke accepts one same-owner opaque key name. These root operations do not require an API key or inspect `manage_api_keys`.

Friend Group messages are a read-only projection of the group's bound Workspace History. List/get/audio requests accept the authenticated member's local `friend_group_name` and, where needed, the message's `history_name`; the Server resolves them to canonical IDs, verifies membership, and carries only those IDs internally. Each response exposes the record identity as `name` and attribution as `actor_name`. Conversation is the only write path. Audio get uses the standard metadata, binary frames, and EOS response and never exposes canonical group, Workspace, or asset locators.

`server.peer.delete` has empty request and response messages and never accepts a target public key. It atomically creates or reuses the caller's pending-deletion handoff while retaining the active Peer, then the Server immediately marks the current connection retiring and rejects new work, and attempts to flush the response and EOS. The full connection closes even if either write fails. `server.workspace.delete` creates or reuses the same transparent handoff only for a caller-owned user Workspace; system Workspaces remain non-deletable. `server.pet.delete` retains the Pet and writes or reuses Pet pending work while retaining its bound system Workspace.
