# @gizclaw/gizclaw

Browser-side WebRTC helpers for GizClaw peer sessions.

## What This Package Provides

- WebRTC signaling helpers for the peer HTTP `/webrtc/v1/offer`
  endpoint.
- GizClaw RPC calls over the `giznet/v1/service/0` data channel using the
  same framed `rpcapi` envelope as the Go client.
- Workspace-related RPC convenience methods.
- A fetch-compatible adapter boundary for generated-client requests. Admin API
  generated-client integration is completed by the desktop Admin UI work.

## Signaling Surfaces

Use `connectGiznetWebRTC` for browser or desktop frontend sessions that connect
to a GizClaw server endpoint. It targets the peer HTTP endpoint described
by `api/peer_http.json`:

```text
POST /webrtc/v1/offer
Content-Type: application/octet-stream
X-Giznet-Public-Key: <peer public key>
X-Giznet-Timestamp: <unix timestamp>
X-Giznet-Nonce: <base64url nonce>
```

The request body is encrypted SDP offer bytes. The response body is encrypted
SDP answer bytes.

`connectGiznetWebRTC` prepares the peer connection by creating the
`giznet/v1/packet` data channel and an Opus-capable audio transceiver before
creating the SDP offer. Callers still provide the crypto/signaling hooks, so
browser, Wails, and Node runtimes can inject their own identity and fetch
primitives.

## RPC Data Channel

GizClaw RPC uses one ordered data channel per request:

```text
giznet/v1/service/0
```

Payloads use the Go `rpcapi` frame format:

```text
uint16 payload_length little-endian
uint16 frame_type little-endian
payload bytes
```

Peer RPC request and response envelopes are protobuf binary frames followed by
an EOS frame.

## HTTP Over Data Channel

The current GizClaw WebRTC bridge exposes JSON-RPC over data channels. It is not
a generic HTTP proxy yet. Frontend code can still use generated clients by
passing a custom `fetch` function from `createWebRTCFetch`, but that fetch
function must map each HTTP request to an RPC method.

Full generated Admin API integration over WebRTC is part of the Wails Admin UI
rewrite.
