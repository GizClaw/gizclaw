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
| `Server.DeleteSelf` | Atomically create or reuse a durable pending-deletion handoff for the authenticated Peer. |

Public key is Peer identity and should not be mixed with database ID, connection ID or Edge assignment. WebRTC connection lifecycle belongs to `giznet` and root `PeerManager`, and does not belong to this package.

Peer deletion creates or reuses one `kind=peer` PendingDeletion in the same KV transaction while retaining the active record and every Peer index. The marker does not cascade into or change Workspace, Pet, social, gameplay, or RegistrationToken resources, and does not alter Peer reads, authorization, or mutations. Admin deletion does not forcibly close an online connection. `server.peer.delete` is caller- and connection-generation-scoped: a superseded connection is rejected before it can retire the replacement generation. After the durable marker write commits, the root connection runtime immediately enters retiring state, detaches the current online connection and registration, rejects new work, and then attempts to write the acknowledgement and EOS; it closes the full Giznet connection even if either write fails. A lost acknowledgement reconnect reuses the retained Peer without creating another pending event. Configured Edge bootstrap, generic writes, and registration-owned firmware binding remain available while the locator is pending.
