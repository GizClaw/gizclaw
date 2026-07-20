# Server Provided to Client

These methods are implemented by Server and called by a Client/Device through its Peer connection.

## Method groups

| Prefix | Main capabilities |
| --- | --- |
| `server.info.*`, `server.runtime.*`, `server.status.*` | Peer information and runtime status |
| `server.run.*` | Workspace selection, history, memory, speech output, reload and stop |
| `server.workspace.*` | Peer-owned Workspace CRUD and history; list requires Collection |
| `server.workflow.*` | RuntimeProfile Workflow alias list/get; list requires Collection |
| `server.model.*`, `server.voice.*`, `server.tool.*` | Safe RuntimeProfile alias list/get |
| `server.speech.*` | Standalone streaming transcription and synthesis |
| `server.register` | Select the required RuntimeProfile and persist/return the RegistrationToken's optional Firmware release-line ID; channel selection remains device-owned |
| `runtime.adopt`, `server.pet.*`, `server.badge.*`, `server.points.*` | Gameplay and Peer-owned Pet state |
| `server.friend.*`, `server.friend_group.*`, `server.contact.*` | Social state |
| `server.firmware.*` | Compatibility surface only; Admin manages Firmware |

`server.peer.lookup`, `server.peer.assign`, and `server.route.resolve` belong only to Edge-node RPC.

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
