# Management

`Implementation file: peer_manager.go`

`peer_manager.go` Maintain the online peers currently visible to the Server, and provide peer operation portals for other GizClaw components.

| Documentation | Features included |
| --- | --- |
| `peer_manager.go` | Maintain online Peer and connection replacement; connect online, offline and forced disconnection; query connections and Peer runtime; ensure the existence of Peer resources; refresh device hardware, SN, IMEI and labels through Peer RPC; coordinate concurrent updates of telemetry status. |

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

Connection activation reserves its public key under the Manager lock, checks durable Peer availability without holding the global lock, and publishes the exact connection only while that reservation remains current. A pending marker or permanent tombstone fails activation; a reconnect waiting behind self-delete neither reuses the retained record nor creates a new generation. A reservation without a published connection is offline. An existing generation stays available while its replacement is being ensured; forced offline clears that generation without discarding the replacement reservation, and transport service loops start only after the new connection is published. Connection-scoped self-delete first publishes deleting state and then commits the durable marker. After commit, Manager quiesces the identity, and replacement activation, registration, and server-initiated Peer RPC remain rejected by the durable fence while unrelated Peers stay available.

## Device metadata ownership

After a Peer connection is published, the Server performs one bounded device-information refresh. A failure does not disconnect the Peer, and Admin can still retry with an explicit refresh. `client.info.get` refreshes only `HardwareInfo` (`hardware_revision`, `manufacturer`, and `model`). `client.identifiers.get` refreshes `DeviceIdentifiers` (`sn`, `imeis`, and `labels`). SN is an optional weak identifier declared by the Client; it must be valid UTF-8 and at most 256 bytes. A Client should keep it stable and as unique as practical for one physical device, but the Server does not treat SN as a unique identity. Several Peers may report the same value, and the Admin SN query returns every match. The server-owned profile fields `name` and `emoji` are changed through `server.info.put` and are not overwritten by reverse refresh. Names must be valid UTF-8 and at most 256 bytes; emoji values must be valid UTF-8 and at most 64 bytes.

Friends read these text profile fields through `server.friend.info.get`. The method requires an existing caller-scoped friend relation and returns no binary avatar data.
