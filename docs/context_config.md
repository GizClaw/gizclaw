# GizClaw CLI Context Config

The GizClaw CLI stores each context in a context directory with a `config.yaml`
file. The context config describes the local identity and one WebRTC-only
GizClaw server endpoint.

## Example

```yaml
description: Local development server
identity:
  private-key: <client-private-key>
server:
  endpoint: 127.0.0.1:9820
```

## Fields

- `description` is optional display metadata for context pickers and desktop
  launchers.
- `identity.private-key` is the local client private key for this context.
- `server.endpoint` is the server `host:port` value without a URL scheme.

## Transport Behavior

Contexts use the single configured endpoint for server-public HTTP, WebRTC
signaling, and WebRTC ICE:

```text
http://server.endpoint/server-info
http://server.endpoint/webrtc/v1/offer
server.endpoint over UDP for WebRTC ICE
```

The client fetches `http://server.endpoint/server-info` before dialing. The
server-info response provides the server public key and the signaling path. The
signaling path is not stored in the context config.

```text
/webrtc/v1/offer
```
