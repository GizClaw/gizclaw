# Server Provided to Client

These methods are implemented by Server and called by a Client/Device through its Peer connection.

The [RPC API Reference](/references/rpc) is the single list of exact method IDs, names, groups, and purposes. This page only explains Server-provided resource projection, call flow, and implementation ownership. See [Server Provided to Edge-node](./server-provided-to-edge-node) for the authorization boundary of Edge-node-only methods.

## RuntimeProfile resource projection

Canonical Workflow, Model, Credential, Voice, and Tool resources are Admin-managed. Peer RPC has no Workflow, Model, Credential, or Tool create/put/delete methods and no `source=runtime|owned` selector.

RuntimeProfile binding aliases are grouped under Collections, but the Peer boundary projects each binding as an immutable `name`. `server.workflow.list` requires a Collection; Workflow, Model, Voice, and Tool get/list requests and responses consistently use names. Responses use `runtime_profile_name`; because RuntimeProfile has no separate Peer alias, its value is the canonical RuntimeProfile ID projected verbatim as the Peer name, alongside the revision. This is the normal Peer projection rule, not a compatibility field. Other canonical IDs, provider configuration, credentials, ownership, and executor routing stay on the Server.

`server.workspace.input.put` updates only the input mode of one Workspace. The Client sends the Workspace `name` and the target `WorkspaceInputMode`; the Server reads the current Workspace, resolves the inherited parameters variant from the Workflow driver, replaces `input` only, and stores the result. Every other parameters field and the toolkit policy keep their stored value. The Client must not GET the Workspace or its Workflow before writing. A missing Workspace returns 404. A Workspace the caller can resolve only through a Friend, Friend Group or Pet relationship but does not own returns 403, while an unshared Workspace owned by another Peer is not disclosed and returns 404. A Workflow driver without an input mode (`dashscope-realtime`, `doubao-realtime-duplex`) or an invalid input returns 400, and the existing system Workspace update limits still apply and answer 409. The read, patch and write happen under one Workspace record lock, so a Workspace update that commits in between is never overwritten.

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

Friend Group messages are a read-only projection of the group's bound Workspace History. List/get/audio download requests accept the authenticated member's local `friend_group_name` and, where needed, the message's `history_name`; the Server resolves them to canonical IDs, verifies membership, and carries only those IDs internally. Each response exposes the record identity as `name` and attribution as `actor_name`. Conversation is the only write path. Audio download uses the standard metadata, binary frames, and EOS response and never exposes canonical group, Workspace, or asset locators.

`server.peer.delete` has empty request and response messages and never accepts a target public key. It atomically creates or reuses the caller's pending-deletion handoff while retaining the active Peer, then the Server immediately marks the current connection retiring and rejects new work, and attempts to flush the response and EOS. The full connection closes even if either write fails. `server.workspace.delete` creates or reuses the same transparent handoff only for a caller-owned user Workspace; system Workspaces remain non-deletable. `server.pet.delete` retains the Pet and writes or reuses Pet pending work while retaining its bound system Workspace.

## Server-initiated device control

The Public HTTP `/gizclaw/v1/device*` control routes make the Server the caller: over the API key owner's active Peer connection it invokes `client.device.status.get` (100), `client.device.volume.set` (101), `client.device.sound.play` (102), `client.device.reboot` (103), `client.wifi.status.get` (104), `client.wifi.saved.list` (105), `client.wifi.saved.forget` (106), `client.wifi.scan` (108), `client.wifi.connect` (109), and `client.firmware.update` (111). Provider responsibilities, idempotency requirements, and error codes for these methods live in [Client Provided to Server](./client-provided-to-server). Server-side rules: each command uses its own RPC stream with a 5-second timeout except scan, which clamps the request to 1–15 seconds; commands for one owner are forwarded serially in arrival order and are never merged or replayed; the `PeerStatus` returned by `volume.set` is stored as the owner's status with the device's reported time, so `server.status.get` and `GET /device/status` then read the same data; after a device acknowledges `reboot`, `firmware.update`, or `wifi.connect`, later commands on that same connection answer `DEVICE_OFFLINE` until the device reconnects.
