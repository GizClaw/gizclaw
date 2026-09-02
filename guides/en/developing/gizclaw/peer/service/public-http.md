# HTTP Service Entrypoints

`Implementation file: peer_service_serve_peer_http.go`

Provides ordinary Peer Public HTTP and Edge Public HTTP, assembles API key, CORS, OpenAI API, Edge signaling routes, and the `/gizclaw/v1/device*` and `/gizclaw/v1/contacts*` device extension, and performs access judgment of Edge client/signaling Peer.

This file has HTTP surface composition; API key state belongs to `services/system/apikey`, and specific API behavior belongs to the corresponding domain service. The device extension handlers live in two files: `peer_service_serve_peer_http_device_api.go` adapts `/device`, `/device/runtime`, `/device/status`, `/device/telemetry*`, and `/contacts*` to `peerresource.DeviceReads` and `services/social/contact`; `peer_service_serve_peer_http_device_control.go` forwards `PUT /device/volume`, `POST /device/actions/*`, and `/device/wifi*` through `deviceController` as `client.device.*` / `client.wifi.*` RPCs and writes the reported `PeerStatus` back through `services/runtime/peertelemetry`.

## Owner binding and ingress

Bearer authentication for `/gizclaw/v1/*` completes before the strict handler: the middleware resolves the API key principal, verifies that the owner is an active Client with a RuntimeProfile binding, and stores the owner public key in the request context (`peerhttp.CallerPublicKey`). Handlers take the owner only from that context; no route accepts a Peer selector, and manager keys and ordinary keys have equal capability on device and contact routes.

Direct Server HTTP (the `server.go` mux, enabled by `serve-to-clients=true`), the Peer Public HTTP service, and Edge HTTPS share one handler; Edge forwards by the `/gizclaw/v1/` prefix and needs no knowledge of the new routes. CORS preflight allows `GET`, `POST`, `PUT`, `DELETE`, and `OPTIONS`.

## Server-to-device RPC forwarding

`deviceController` locates the owner's active connection through `callPeerRPC`, opens one RPC stream per command with a 5-second timeout, and forwards commands for one owner serially in arrival order. No connection answers `409 DEVICE_OFFLINE`, a timeout answers `504 DEVICE_TIMEOUT`, a device `INVALID_PARAMS` answers `400 DEVICE_REJECTED`, `METHOD_NOT_FOUND` answers `501 DEVICE_UNSUPPORTED`, and every other RPC error answers a redacted `502 DEVICE_ERROR`. After a device acknowledges `reboot`, the controller remembers the acknowledging connection and answers later commands with `409 DEVICE_OFFLINE` until that connection is replaced. The `volume.set` response is written through `StatusSync.ApplyDeviceStatus` under the owner's telemetry status lock, keeping per-field time ordering with telemetry reports.

When a browser request carries `Origin`, Direct Server, Peer Public HTTP, and Edge Public HTTP return that actual origin and append `Origin` to `Vary` to isolate caches; non-browser requests retain `*` compatibility. An `OPTIONS` preflight for a supported path returns `204` directly and allows the Public HTTP methods `GET`, `POST`, `DELETE`, and `OPTIONS`, together with the headers used by API keys, signaling, and request correlation.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `servePublic` / `serveEdgePublic` | Start normal or Edge Public HTTP on the corresponding Giznet service. |
| `publicHTTPHandlerWithOptions` | Assemble API key management, device extension, and signaling routes, and complete owner binding. |
| `deviceController` | Serialize device control commands, map offline/timeout/device errors, and write back `PeerStatus`. |
| `deviceReadsForAPIKey` | Build the read-only `peerresource.DeviceReads` for an API key owner. |
| `allowEdgeClientPeer` | Determine whether the Peer is allowed to serve as an Edge client. |
| `allowEdgeSignalingPeer` | Determine whether the Peer is allowed to initiate signaling through the Edge. |
| `setPeerHTTPCORSHeaders` | Set the CORS headers of the Peer HTTP surface. |
