# GizClaw CLI Context Config

The GizClaw CLI stores each context in a context directory with a `config.yaml`
file and an `identity.key` file. The context config describes how the CLI dials
one GizClaw server.

## Example

Noise context:

```yaml
server:
  host: 127.0.0.1
  public-api-port: 9820
  noise-udp-port: 9820
  public-key: <server-public-key>
  transport: noise
  cipher-mode: chacha_poly
```

WebRTC context:

```yaml
server:
  host: 127.0.0.1
  public-api-port: 9820
  noise-udp-port: 9820
  ice-port: 9821
  public-key: <server-public-key>
  transport: webrtc
  cipher-mode: chacha_poly
```

## Fields

- `server.host` is the server host or IP address without a port.
- `server.public-api-port` is the HTTP public API port. WebRTC signaling also
  uses this port.
- `server.noise-udp-port` is the Noise-over-UDP transport port.
- `server.ice-port` is the WebRTC ICE UDP/TCP port. It is required for
  `transport: webrtc`.
- `server.public-key` is the server static public key and is the trust anchor
  for the context.
- `server.transport` selects the dial transport. Valid values are `noise` and
  `webrtc`.
- `server.cipher-mode` selects the configured transport/signaling cipher mode.
  Valid values are `chacha_poly`, `aes_256_gcm`, `plaintext`, or empty.

## Transport Behavior

Noise contexts dial:

```text
host:noise-udp-port over UDP
```

WebRTC contexts send a sealed HTTP offer to the fixed signaling path:

```text
http://host:public-api-port/giznet/webrtc/v1/offer
```

Then WebRTC ICE uses:

```text
host:ice-port over UDP and passive ICE-TCP
```

The WebRTC signaling path is fixed by the protocol and is not stored in the
context config:

```text
/giznet/webrtc/v1/offer
```

