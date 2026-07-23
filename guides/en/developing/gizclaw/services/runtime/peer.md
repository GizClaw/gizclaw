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
| `Server.EnsureConnectedPeer` / `EnsureConnectedPeerGuarded` | Create a default active Peer for the authenticated public key; the guarded form revalidates connection lifecycle state under the per-record lock before reading or creating it. |
| `Server.LoadPeer` / `SavePeer` | Press public key to read or save the complete Peer. |
| `Server.BootstrapEdgeNodes` | Synchronize the Edge Node identity in the configuration as a Peer resource. |
| `Server.DeleteSelf` | Atomically remove the authenticated Peer and write its durable pending-deletion handoff. |

Public key is Peer identity and should not be mixed with database ID, connection ID or Edge assignment. WebRTC connection lifecycle belongs to `giznet` and root `PeerManager`, and does not belong to this package.

Peer deletion removes the active record and every Peer index in the same KV transaction that writes one `kind=peer` PendingDeletion. It does not cascade into Workspace, Pet, social, gameplay, or RegistrationToken resources. Admin deletion does not forcibly close an online connection. `server.peer.delete` is caller- and connection-generation-scoped: a superseded connection is rejected before it can delete the replacement generation. After the durable deletion commits, the root connection runtime immediately enters retiring state, detaches the current online connection and registration, rejects new work, and then attempts to write the acknowledgement and EOS; it closes the full Giznet connection even if either write fails. A lost-acknowledgement reconnect may create a later Client record without overwriting the older pending event. The deletion retry's pending lookup and reconnect create are serialized by the same per-record lock; an activation already waiting behind deletion must also pass its Manager reservation guard under that lock before it can create a record. Configured Edge bootstrap and generic writes remain blocked while the locator is pending, while registration-owned firmware binding may update an active reconnected generation.
