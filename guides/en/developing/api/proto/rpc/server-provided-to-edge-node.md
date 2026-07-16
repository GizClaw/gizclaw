# Server Provided to Edge-node

This set of capabilities is implemented by Server and is only provided to connections with Edge-node role. Edge-node uses it to query Peer assignments and resolve upstream routes, without exposing control plane capabilities to ordinary clients.

## Methods

| Method | Function |
| --- | --- |
| `server.peer.lookup` | Query the current assignment of the specified Peer |
| `server.peer.assign` | Create or update assignment for the specified Peer and perform version conflict check |
| `server.route.resolve` | Resolve available route/assignment for the target Peer |

## Calling relationship

```mermaid
sequenceDiagram
    participant Edge as Edge-node
    participant Server as Server Edge RPC
    participant Route as Peer Route service
    Edge->>Server: lookup / assign / resolve
    Server->>Route: validate and query assignment
    Route-->>Server: assignment / route error
    Server-->>Edge: typed response / RPC error
```

Server uses independent Edge RPC dispatch and only accepts the above three methods. Even if the ordinary Client RPC surface shares the same `rpc.proto` registry, it cannot obtain the calling permission because the method can be decoded; role authorization and service exposure must limit the Edge control plane at the same time.
