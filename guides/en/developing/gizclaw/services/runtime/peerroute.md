# Peer Route

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peerroute)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute)

`peerroute` stores the fixed home-Server assignment of each Client Peer in the configured shared Peer Store. Every Server in one topology can read this control-plane directory; it does not provide automatic reassignment, Server failover, Workspace ownership, or Workspace routing.

`Lookup` and `Resolve` are read-only. `Assign` creates version 1 only when the assignment is absent. The same Server owner may atomically refresh endpoint or role metadata and increment the version; another Server always receives a conflict and cannot transfer ownership, including when it supplies `expected_version`.

Client activation performs this claim or verification before the Manager publishes the connection. A foreign-home direct or Edge-tunneled connection is rejected before RPC, HTTP, packet, audio, Agent, or PeerRun work starts. Route assignment, an online connection, and a persisted Peer remain separate states; an assignment does not imply that its owner Server or Peer is currently reachable.

Assignments and Peer admission/deletion fences use [strong KV reads](../../../stores/kv), even when the connector enables Redis replica or TiKV regional Learner stale reads. Route resolve/assign checks the pending-deletion fence before loading Peer authority; a stale active replica cannot authorize a deleted Peer.
