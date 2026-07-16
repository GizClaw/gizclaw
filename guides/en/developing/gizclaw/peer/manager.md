# Management

`Implementation file: peer_manager.go`

`peer_manager.go` Maintain the online peers currently visible to the Server, and provide peer operation portals for other GizClaw components.

| Documentation | Features included |
| --- | --- |
| `peer_manager.go` | Maintain online Peer and connection replacement; connect online, offline and forced disconnection; query connections and Peer runtime; ensure the existence of Peer resources; refresh devices, hardware, IMEI and labels through Peer RPC; coordinate concurrent updates of telemetry status. |

This prefix has server-perspective online connection indexing and cross-connection operations, but does not have a peer persistence model. The Peer resource itself belongs to `services/runtime/peer`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`Manager`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager) | Aggregate domain services and maintain an index of public key to online connection. |
| [`NewManager`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#NewManager) | Create Manager and set up Peer service. |
| `activePeer` | Stores the currently active connection of a single Peer. |
| [`Manager.SetPeerUp`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.SetPeerUp) / [`SetPeerDown`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.SetPeerDown) / [`ForcePeerDown`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.ForcePeerDown) | Manage connection online, conditional offline and forced offline. |
| `allowService` / `allowActivePeerRole` | Determine Giznet service admission based on Peer role. |
| [`Manager.Peer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.Peer) / [`PeerRuntime`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.PeerRuntime) | Query online connection or runtime snapshot. |
| [`Manager.EnsurePeer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.EnsurePeer) | Ensure that the persistent Peer resource exists. |
| [`Manager.RefreshPeer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.RefreshPeer) / `refreshPeer` | Via Peer RPC Pull device information and write changes back to the Peer resource. |
| `peerRPCConn` / `callPeerRPC` | Open Peer RPC stream and execute typed RPC call. |
| `retainTelemetryStatusLock` / `releaseTelemetryStatusLock` | Manage telemetry status and update the lock life cycle by public key. |
| `applyPeerRefreshInfo` / `applyPeerRefreshIdentifiers` | Merge RPC refresh response into the persistent Peer model. |
