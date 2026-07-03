# GizClaw CLI Context Config

The GizClaw CLI stores each context in a context directory with a `config.yaml`
file and an `identity.key` file. The context config describes one WebRTC-only
GizClaw server endpoint.

## Example

```yaml
description: Local development server
server:
  endpoint: 127.0.0.1:9820
  public-key: <server-public-key>
```

## Fields

- `description` is optional display metadata for context pickers and desktop
  launchers.
- `server.endpoint` is the server `host:port` value without a URL scheme.
- `server.public-key` is the server static public key and is the trust anchor
  for the context.

## Transport Behavior

Contexts use the single configured endpoint for server-public HTTP, WebRTC
signaling, and WebRTC ICE:

```text
http://server.endpoint/server-info
http://server.endpoint/webrtc/v1/offer
server.endpoint over UDP for WebRTC ICE
```

The WebRTC signaling path is fixed by the protocol and is not stored in the
context config:

```text
/webrtc/v1/offer
```
