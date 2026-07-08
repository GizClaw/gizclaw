# GizClaw Event And Media Streams

This document covers non-service stream contracts. `Service*` names are reserved
for RPC and HTTP surfaces.

## Agent Event Stream

`EventStreamAgent` is a reliable event stream carried over giznet service stream
ID `0x20`.

It carries framed agent lifecycle, text, control, and workspace-history events.
It is reliable because the agent event stream uses the same ordered service
stream carrier as other giznet service streams, but it is not named `Service*`
because its semantic contract is an event stream.

## Telemetry Event Stream

`EventStreamTelemetry` is an unreliable application/custom direct packet
protocol byte `0x40`.

It carries peer/device telemetry samples encoded from
`api/telemetry/peer_telemetry.proto`. Telemetry is an event stream conceptually,
but the carrier is the direct packet channel, not a reliable service stream.

## Opus Media Stream

`MediaStreamOpus` identifies the WebRTC Opus media stream with codec
`audio/opus`.

Opus audio uses the WebRTC media channel. It is not a service and not an event
stream.

## Stamped Opus Packet Bridge

`ProtocolStampedOpusPacket` is the giznet well-known direct packet protocol byte
`0x10`. `PacketStampedOpus` is retained as a compatibility alias in peer-facing
code.

The bridge maps WebRTC RTP Opus payloads to stamped Opus packets toward the
peer, and maps peer stamped Opus packets back to the WebRTC audio track. This
packet bridge exists to connect the media channel to peer packet delivery; it
does not rename or replace `MediaStreamOpus`.
