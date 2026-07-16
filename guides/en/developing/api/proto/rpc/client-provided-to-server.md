# Client Provided to Server

This set of capabilities is implemented by Client/Device and called by Server on Peer connection. Server uses it to read the device's own information or request the device to perform local capabilities.

## Methods

| Method | Function |
| --- | --- |
| `client.info.get` | Read Client current device information |
| `client.identifiers.get` | Read Client hardware/device identifiers |
| `client.tool.invoke` | Request Client to execute its locally provided Tool |

## Calling relationship

```mermaid
sequenceDiagram
    participant Server
    participant Client
    Server->>Client: client.* request
    Client->>Client: Read device state or invoke local tool
    Client-->>Server: typed response / RPC error
```

A Client provider can only return data that is owned or executable by the Client. Server resources, ACL decisions, cross-peer lookup and persistence management cannot be implemented as `client.*`.

Go Client's provider dispatch is located at `sdk/go/gizcli`'s RPC Client implementation; the server side calls these methods through online Peer connection.
