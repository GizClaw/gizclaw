# Peer

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer)

`peer` Owns server-side persistent Peer resources and implements Peer CRUD, verification, indexing and connected-peer bootstrap required for Admin HTTP and Peer HTTP.

## Core structure and main function

| Structure or function | Function |
| --- | --- |
| `Server` | Combines Peer store, online `PeerManager` and HTTP service dependencies. |
| `PeerManager` | Query online Peer connection/runtime, does not have persistent records. |
| `PeerAdminService` | Define the Peer operations required by the Admin surface. |
| `PeerHTTPService` | Define the Peer operations required for Peer-facing surface. |
| `Server.EnsureConnectedPeer` | Create a default active peer for the authenticated public key. |
| `Server.LoadPeer` / `SavePeer` | Press public key to read or save the complete Peer. |
| `Server.BootstrapEdgeNodes` | Synchronize the Edge Node identity in the configuration as a Peer resource. |

Public key is Peer identity and should not be mixed with database ID, connection ID or Edge assignment. WebRTC connection lifecycle belongs to `giznet` and root `PeerManager`, and does not belong to this package.
