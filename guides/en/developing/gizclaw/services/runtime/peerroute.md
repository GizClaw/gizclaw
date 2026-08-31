# Peer Route

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute)

`peerroute` stores the fixed home-Server assignment of each Client Peer in the configured shared Peer Store. Every Server in one topology can read this control-plane directory; it does not provide automatic reassignment, Server failover, Workspace ownership, or Workspace routing.

## Core structure and main function

| Structure or function | Function |
| --- | --- |
| `Server` | Provides read-only lookup/resolve and atomic fixed-owner claim/refresh. |
| `PeerStore` | Read the Peer resource associated with the assignment. |
| `ParsePublicKey` | Verify wire/string public key. |
| `ToRPC` | Convert internal `PeerAssignment` to RPC message. |

`Lookup` and `Resolve` are read-only. `Assign` creates version 1 only when the assignment is absent. The same Server owner may atomically refresh endpoint or role metadata and increment the version; another Server always receives a conflict and cannot transfer ownership, including when it supplies `expected_version`.

Client activation performs this claim or verification before the Manager publishes the connection. A foreign-home direct or Edge-tunneled connection is rejected before RPC, HTTP, packet, audio, Agent, or PeerRun work starts. Route assignment, an online connection, and a persisted Peer remain separate states; an assignment does not imply that its owner Server or Peer is currently reachable.
