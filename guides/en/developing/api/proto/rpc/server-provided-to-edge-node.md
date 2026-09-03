# Server Provided to Edge-node

This set of capabilities is implemented by Server and is only provided to connections with Edge-node role. Edge-node uses it to query Peer assignments and resolve upstream routes, without exposing control plane capabilities to ordinary clients.

The [RPC API Reference](/references/rpc#edge-rpc) is the single list of exact method IDs, names, and purposes. This page only explains the Edge-node role, call flow, and authorization boundary.

## Calling relationship

```mermaid
sequenceDiagram
    participant Edge as Edge-node
    participant Server as Server Edge RPC
    participant Route as Peer Route service
    Edge->>Server: lookup / assign / resolve / API key resolve
    Server->>Route: authenticate when needed, then query assignment
    Route-->>Server: assignment / route error
    Server-->>Edge: typed response / RPC error
```

Server uses independent Edge RPC dispatch and accepts only these route methods. Even if the ordinary Client RPC surface shares the same `rpc.proto` registry, it cannot obtain the calling permission because the method can be decoded; role authorization and service exposure must limit the Edge control plane at the same time.

`server.api_key.resolve` lets the Edge submit an incoming Bearer credential to
the Server API-key service and obtain the owner Peer's fixed assignment. In a
multi-Server deployment, Servers share the protected API-key Redis Store; the
Edge receives no Redis credential. The RPC returns only the assignment, never
the API-key record, hash, or secret index, and errors and logs must not contain
the request credential. The target Server still performs final authentication
and authorization for the original HTTP request.
