# Edge Routing

## Cross-Server notifications

`edge.social.ping.relay` (149) is provided by Edge to its configured,
identity-verified upstream Servers. `server.social.ping.deliver` (150) is
provided by Server to active Edges. Both use physical `ServiceEdgeRPC` (0x31)
and `SocialPingDeliveryRequest`: target PK, `ClientSocialPingRequest` and
`deadline_unix_ms`. The empty `SocialPingDeliveryResponse` succeeds only
after the device ACK. The device still receives the existing `client.social.ping`.

Manager prefers the local Peer, otherwise selects a connected trusted Edge in
PK order without switching relay after sending. Edge registers reverse handlers
on outbound control and gateway pool Server connections, never device ingress.
It resolves the existing assignment and sends one deliver to the matching
configured Server. The target rechecks the active Edge role and delivers locally
only, preventing loops. Offline targets return NOT_FOUND, timeouts
DEADLINE_EXCEEDED, and unavailable transport or relay UNAVAILABLE. Social
adapters count only device ACKs and retain public results and window release.

Every hop preserves the deadline and request correlation ID, with a local
three-second cap. Success, cancellation, disconnection and timeout close the
request stream; the existing lifecycle owns the parent connection. Concurrent
upstream reconnects share one cancelable dial without network I/O under the mutex.

Source：[rpc_edge.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_edge.go)

Define `edgeRPCServer`, process Peer lookup, assignment, route resolve, and API-key
owner route resolve on the Edge Giznet service; uniformly encode RPC results,
and map API-key, `peerroute`, Peer, and KV errors to RPC error codes.

`server.peer.lookup` and `server.route.resolve` are read-only. `server.peer.assign` atomically claims a missing Client Peer for the current Server, returns the existing assignment for the same owner, and may refresh only that owner's endpoint/role metadata. A different Server owner returns conflict and is never overwritten; `expected_version` detects stale updates but never authorizes ownership transfer.

`server.api_key.resolve` (method ID 99) is exposed only through the authenticated
Edge RPC service. The request carries the Bearer credential. The Server uses its
own `services.api_key.store` to authenticate it, obtains the owner Peer from the
API-key record, and returns that Peer's existing `PeerAssignment`. It never
creates, moves, or refreshes an assignment. Invalid or revoked credentials map
to forbidden; a missing or inactive owner route maps to not found. Errors never
echo the credential.
