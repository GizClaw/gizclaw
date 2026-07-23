# Server Provided to Client

These methods are implemented by Server and called by a Client/Device through its Peer connection.

The [RPC API Reference](/references/rpc) is the single list of exact method IDs, names, groups, and purposes. This page only explains Server-provided resource projection, call flow, and implementation ownership. See [Server Provided to Edge-node](./server-provided-to-edge-node) for the authorization boundary of Edge-node-only methods.

## RuntimeProfile resource projection

Canonical Workflow, Model, Credential, Voice, and Tool resources are Admin-managed. Peer RPC has no Workflow, Model, Credential, or Tool create/put/delete methods and no `source=runtime|owned` selector.

Workflow aliases are grouped under RuntimeProfile Collections. `server.workflow.list` requires a Collection; `server.workflow.get` uses the globally unique alias. Model, Voice, and Tool list/get also address RuntimeProfile aliases. Responses contain only safe alias metadata and include the RuntimeProfile name and revision; canonical IDs, provider configuration, credentials, ownership, and executor routing stay on the Server.

Workspace create requires `collection` and `workflow_alias`. The Server records Collection through an internal Workspace label. Workspace list requires Collection and performs exact filtering, but generic labels are not part of the Peer response. Removing an alias does not hide or delete an existing Workspace; reload/run reports not found until the alias exists again.

## Calling relationship

```mermaid
sequenceDiagram
    participant Client
    participant RPC as Server RPC
    participant Profile as RuntimeProfile snapshot
    participant Service as Domain service
    Client->>RPC: typed request
    RPC->>Profile: resolve aliases and policy
    RPC->>Service: typed command/query
    Service-->>RPC: result / domain error
    RPC-->>Client: typed response / frames / RPC error
```

The RPC adapter owns payload decoding, framing, lifecycle, and stable error mapping. Domain services own storage, resource validation, authorization, and execution.

`server.peer.delete` has empty request and response messages and never accepts a target public key. After atomically removing the caller's active Peer and writing its pending-deletion handoff, the Server immediately marks the current connection retiring and rejects new work, then attempts to flush the response and EOS. The full connection closes even if either write fails. `server.workspace.delete` performs the same fast handoff only for a caller-owned user Workspace; system Workspaces remain non-deletable. `server.pet.delete` removes the Pet and writes Pet pending work while retaining its bound system Workspace.
