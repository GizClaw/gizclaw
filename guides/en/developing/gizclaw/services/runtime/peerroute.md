# Peer Route

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute)

`peerroute` Maintains the Peer assignments known to the current Server and provides query and update capabilities for Edge/Server routing. It describes the control plane status of this server and does not mean that mesh-wide directory or cross-server automatic synchronization already exists.

## Core structure and main function

| Structure or function | Function |
| --- | --- |
| `Server` | Provides assignment reading, writing and RPC handlers. |
| `PeerStore` | Read the Peer resource associated with the assignment. |
| `ParsePublicKey` | Verify wire/string public key. |
| `ToRPC` | Convert internal `PeerAssignment` to RPC message. |

Route assignment, Peer online connection and persistent Peer are three different states. Code cannot infer that the target is currently online just because the assignment exists.
