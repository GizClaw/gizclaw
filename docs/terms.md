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
| Stamped Opus packet bridge | Internal direct packet bridge identified by `PacketStampedOpus`. |

## Constant Names

```text
ServicePeerRPC        = 0x00
ServicePeerHTTP       = 0x01
ServicePeerOpenAI     = 0x02
ServiceAdminHTTP      = 0x10
EventStreamAgent      = 0x20
EventStreamTelemetry  = 0x11
MediaStreamOpus       = "audio/opus"
PacketStampedOpus     = 0x10
```

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
| `ProtocolStampedOpus` | `PacketStampedOpus` |
| `ProtocolEvent` | Removed |
| `serverpublic` | `peerhttp` |
| `adminservice` | `adminhttp` |
| `openaiservice` | `openaihttp` |
| `cmd/internal/publicapi` | `cmd/internal/peerapi` |
| `api/server_public.json` | `api/peer_http.json` |
| `api/admin_service.json` | `api/admin_http.json` |
