# Peer Route

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peerroute)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute)

`peerroute` stores the fixed home-Server assignment of each Client Peer in the configured shared Peer Store. Every Server in one topology can read this control-plane directory; it does not provide automatic reassignment, Server failover, Workspace ownership, or Workspace routing.

`Lookup` and `Resolve` are read-only. `Assign` creates version 1 only when the assignment is absent. The same Server owner may atomically refresh endpoint or role metadata and increment the version; another Server always receives a conflict and cannot transfer ownership, including when it supplies `expected_version`.

Client activation performs this claim or verification before the Manager publishes the connection. Edge-tunneled clients also perform the fixed-home admission before the tunnel acceptance result; a foreign owner is returned to the Edge for bounded redirection to a configured Server. The observed assignment accompanies the ownership conflict, avoiding a second potentially missing reader lookup. Direct foreign-home connections remain rejected. RPC, HTTP, packet, audio, Agent, and PeerRun work start only after the normal connection activation. Route assignment, an online connection, and a persisted Peer remain separate states; an assignment does not imply that its owner Server or Peer is currently reachable.

Assignments and Peer admission/deletion fences follow the [configured KV read policy](../../../stores/kv). Ordinary queries accept stale routes, active status and missing results without a primary refresh. With no explicit version, `Assign` reuses a matching existing home without a writer transaction. Missing assignments are claimed atomically; metadata changes and explicit-version no-ops compare writer conditions. A rejected condition permits bounded refresh. When activation has just confirmed a Peer but route admission cannot see it, Manager refreshes only that bootstrap update. These paths never transfer ownership or relax an explicit version.
