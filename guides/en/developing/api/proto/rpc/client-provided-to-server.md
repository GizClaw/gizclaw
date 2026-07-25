# Client Provided to Server

This set of capabilities is implemented by Client/Device and called by Server on Peer connection. Server uses it to read the device's own information or request the device to perform local capabilities.

The [RPC API Reference](/references/rpc) is the single list of exact method IDs, names, and purposes. This page only explains the `client.*` provider direction and ownership.

## Calling relationship

```mermaid
sequenceDiagram
    participant Server
    participant Client
    Server->>Client: client.* request
    Client->>Client: Read device state or invoke local tool
    Client-->>Server: typed response / RPC error
```

A Client provider can only return data that is owned or executable by the Client. Server resource-access decisions, cross-peer lookup, and persistence management cannot be implemented as `client.*`.

Go Client's provider dispatch is located in the `sdk/go/gizcli` RPC Client implementation. A C Client registers the same provider direction through `gzc_client_config_t.rpc_provider`; the callback supplies borrowed Protobuf response bytes or a stable RPC error before returning. The server side calls these methods through the online Peer connection.
