# Public API

Public API is an HTTP contract that Server exposes to Public/Peer caller before and after WebRTC connection is established. It is the entry boundary and does not represent the full capabilities of the Peer domain service.

Source:`api/http/peer.json`
Go generated output: `pkgs/gizclaw/api/peerhttp`

See the [API Reference](/api/) for exact endpoints, parameters, requests, and responses. This page only explains the Public/Peer surface design boundary.

`/webrtc/v1/offer` Occurs before the Peer connection is established, HTTP signaling must be preserved. The Peer capability after establishing a connection can use reliable HTTP-over-service-stream or Peer RPC; when choosing a transport, avoid maintaining two sets of contracts for the same capability.

The identity authentication of the Offer is completed by the signing signaling contract itself and should not additionally rely on the Public login session. Public API can reuse real shared types such as `ErrorResponse`, `DeviceInfo` and `Runtime`, but does not reference Admin Resources.

See [Peer HTTP · Side Control](../../gizclaw/peer/service/side-control) for the route contract, session boundary, and transports. LiteLink-local capabilities such as device passwords, Wi-Fi provisioning, and playing sounds are not Public API routes.
