# Public API

Public API is an HTTP contract that Server exposes to Public/Peer caller before and after WebRTC connection is established. It is the entry boundary and does not represent the full capabilities of the Peer domain service.

Source:`api/http/peer.json`
Go generated output: `pkgs/gizclaw/api/peerhttp`

## Endpoints

| Endpoint | Function |
| --- | --- |
| `POST /login` | Establish a primary session or exchange a device token for a Side Control session |
| `GET /server-info` | Query Server public information |
| `POST /webrtc/v1/offer` | Submit signed Offer and get WebRTC Answer |
| `GET /me` | Query the current Peer registration/self view |
| `GET /me/status` | Query current Peer status |
| `PUT /me/status` | Update current Peer status |
| `GET /me/runtime` | Query the current Peer runtime |
| `/me/side-control/*` | Manage primary-device grants and Side Control sessions |
| `/side-control/*` | Query or control the target bound to a Side Control session |

`/webrtc/v1/offer` Occurs before the Peer connection is established, HTTP signaling must be preserved. The Peer capability after establishing a connection can use reliable HTTP-over-service-stream or Peer RPC; when choosing a transport, avoid maintaining two sets of contracts for the same capability.

The identity authentication of the Offer is completed by the signing signaling contract itself and should not additionally rely on the Public login session. Public API can reuse real shared types such as `ErrorResponse`, `DeviceInfo` and `Runtime`, but does not reference Admin Resources.

See [Peer HTTP · Side Control](../../gizclaw/peer/service/side-control) for the route contract, session boundary, and transports. LiteLink-local capabilities such as device passwords, Wi-Fi provisioning, and playing sounds are not Public API routes.
