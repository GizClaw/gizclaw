# @gizclaw/webrtc

Browser-side WebRTC helpers for GizClaw peer sessions.

## What This Package Provides

- WebRTC signaling helpers for the server public `/webrtc/v1/offer`
  endpoint.
- JSON-RPC calls over the `rpc:*` data channel.
- Workspace-related RPC convenience methods.
- A fetch-compatible adapter that can route generated-client requests through
  JSON-RPC.

## Signaling Surfaces

Use `connectGiznetWebRTC` for browser or desktop frontend sessions that connect
to a GizClaw server endpoint. It targets the server public endpoint described
by `api/server_public.json`:

```text
POST /webrtc/v1/offer
Content-Type: application/octet-stream
X-Giznet-Public-Key: <peer public key>
X-Giznet-Timestamp: <unix timestamp>
X-Giznet-Nonce: <base64url nonce>
```

The request body is encrypted SDP offer bytes. The response body is encrypted
SDP answer bytes.

## HTTP Over Data Channel

The current GizClaw WebRTC bridge exposes JSON-RPC over data channels. It is not
a generic HTTP proxy yet. Frontend code can still use generated clients by
passing a custom `fetch` function from `createWebRTCFetch`, but that fetch
function must map each HTTP request to an RPC method.

Full Admin API over WebRTC needs a server-side HTTP-over-data-channel bridge.
