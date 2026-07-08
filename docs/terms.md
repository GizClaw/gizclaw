# GizClaw Terms

## Canonical Terms

| Term | Meaning |
| --- | --- |
| Service | RPC or HTTP surface carried over a reliable giznet service stream. |
| EventStream | Event-oriented stream. The carrier can be reliable or unreliable. |
| MediaStream | Media carried over the WebRTC media channel. |
| Peer RPC surface | Bidirectional RPC surface on `ServicePeerRPC`. |
| Peer HTTP surface | Bootstrap, login, and WebRTC signaling HTTP routes on `ServicePeerHTTP`. |
| Peer OpenAI-compatible HTTP surface | OpenAI-compatible HTTP routes on `ServicePeerOpenAI`. |
| Admin HTTP surface | Admin-only HTTP routes on `ServiceAdminHTTP`. |
| Agent event stream | Reliable framed event stream on `EventStreamAgent`. |
| Telemetry event stream | Unreliable direct packet event stream on `EventStreamTelemetry`. |
| Opus media stream | WebRTC Opus media channel identified by `MediaStreamOpus`. |
| Stamped Opus packet bridge | Giznet well-known direct packet bridge identified by `ProtocolStampedOpusPacket`. |

## Constant Names

```text
ServicePeerRPC        = 0x00
ServicePeerHTTP       = 0x01
ServicePeerOpenAI     = 0x02
ServiceAdminHTTP      = 0x10
EventStreamAgent      = 0x20
EventStreamTelemetry  = 0x40
MediaStreamOpus       = "audio/opus"
ProtocolServiceStream = 0x00
ProtocolStampedOpusPacket = 0x10
```

Giznet direct packet protocol bytes `0x00` through `0x3f` are reserved for
well-known giznet protocols. Values `0x40` through `0xff` are available for
application/custom direct packet protocols.

## Old Name Replacements

| Old name | Canonical name |
| --- | --- |
| `ServiceRPC` | `ServicePeerRPC` |
| `ServiceServerPublic` | `ServicePeerHTTP` |
| `ServiceOpenAI` | `ServicePeerOpenAI` |
| `ServiceAdmin` | `ServiceAdminHTTP` |
| `ServiceAgentStream` | `EventStreamAgent` |
| `ServiceEvent` | `EventStreamAgent` |
| `ProtocolTelemetry` | `EventStreamTelemetry` |
| `ProtocolStampedOpus` | `ProtocolStampedOpusPacket` |
| `PacketStampedOpus` | `ProtocolStampedOpusPacket` |
| `ProtocolEvent` | Removed |
| `serverpublic` | `peerhttp` |
| `adminservice` | `adminhttp` |
| `openaiservice` | `openaihttp` |
| `cmd/internal/publicapi` | `cmd/internal/peerapi` |
| `api/server_public.json` | `api/peer_http.json` |
| `api/admin_service.json` | `api/admin_http.json` |
