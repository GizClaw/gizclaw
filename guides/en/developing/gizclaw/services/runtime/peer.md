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

Peer deletion creates or reuses one `kind=peer` PendingDeletion in the Peer KV and immediately turns the public key into a cross-process identity fence. While the marker exists, Admin Peer get/list and the same delete remain available for diagnostics and idempotent retry. Reconnect, Public login, existing sessions, WebRTC, RPC/streams, Edge bootstrap, business reads, and mutations return `PEER_PENDING_DELETION`. The online connection is quiesced, and a terminal cleanup failure does not lift the fence.

The production handler persists an immutable retirement plan bound to the marker fingerprint and clears owned or selected state through narrow Social, Workspace, Gameplay, Public Login, and RuntimeProfile adapters. Global catalogs/configuration, foreign resources, logs, and metrics are outside deletion scope. Completion uses one guarded KV mutation to remove the Peer payload, secondary indexes, plan, marker, locator, and task and writes the sole `{"version":1,"state":"deleted"}` tombstone at `by-pubkey/<public-key>`. Admin get/list derive `{public_key,status=deleted}` from that sentinel. Every other entry point returns `PEER_DELETED`, and the public key can never register again.
