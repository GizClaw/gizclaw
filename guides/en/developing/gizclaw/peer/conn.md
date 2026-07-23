# Connection

`Implementation files: peer_conn.go, peer_conn_openai.go`

`peer_conn` The prefix has the product-level life cycle of a single Peer connection.

| Documentation | Features included |
| --- | --- |
| `peer_conn.go` | `PeerConn` Main life cycle; accept Giznet service and packet; start normal RPC and Edge RPC; initialize audio mixer, Agent Host, Peer GenX and resource view; process event stream, direct packet, telemetry packet and mixed audio output; close connection-scoped resources uniformly. |
| `peer_conn_openai.go` | Provide OpenAI-compatible HTTP service on the current Peer connection; assemble the RuntimeProfile and owner resource view; expose OpenAI API and voice-list compatibility entry points. |

Universal WebRTC, packet transport and service stream belong to `pkgs/giznet`; universal audio codec belongs to `pkgs/audio`; persistent runtime state belongs to `services/runtime`.

## Transport contract

The [Streams Reference](/references/streams) owns the direction, reliability, service IDs, framing, and lifecycle of audio, direct packets, the Agent Event Stream, and RPC/HTTP service streams. The [Events Reference](/references/events) owns event wire types and fields. This page only explains how `PeerConn` implements those contracts and does not duplicate their protocol tables.

## Service stream write flow control

The JavaScript, Flutter, and C SDKs use one serialized writer per reliable, ordered service DataChannel. Each native DataChannel message carries at most 1400 bytes. A writer pauses at its high-water mark and resumes only after a buffered-amount-low notification reports that the queue reached the low-water mark. Successful completion means every fragment of the logical message was accepted by the local WebRTC send queue; it does not mean the remote peer consumed the message.

JavaScript and Flutter use fixed 1 MiB / 256 KiB high/low water marks. C API v2 defaults to 256 KiB / 64 KiB for embedded callers and allows larger values through `service_write_high_water_bytes` and `service_write_low_water_bytes` in `gzc_client_config_t`; a custom high-water mark must be at least 1400 bytes and the low-water mark must be lower. Synchronous C sends borrow the caller payload only until the call returns and apply `write_timeout_ms` to the complete logical write. Elapsed-time checks use the platform's monotonic `time_instant_ms`, while protocol timestamps continue to use `time_unix_ms`.

Direct packets, Telemetry, and RTP do not use this service stream writer or its water marks.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`PeerConn`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerConn) | Holds Giznet connection, PeerService, RPC Server, Agent Host, audio mixer and connection-scoped services. |
| [`PeerConn.CreateAudioTrack`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerConn.CreateAudioTrack) | Create a track written to the current Peer audio mixer. |
| `serve` | Parallel services Giznet services, direct packets, Agent output and mixed audio. |
| `serveService` | Accept and distribute the Giznet service stream currently opened by Peer. |
| `servePackets` / `serveDirectPackets` | Receive ordinary and direct packets, and distribute telemetry/media. |
| `serveRPC` / `serveEdgeRPC` | Start Peer RPC or Edge RPC service loop. |
| `init` / `initRPC` / `initMixer` / `initAgentHost` / `initPeerGenX` | Assemble connection-scoped runtime dependencies. |
| `serveEvents` / `handleEventStream` | Accept event stream and push Agent input. |
| `processTelemetryPackets` / `handleTelemetryPacket` | Decode telemetry and synchronize Peer status. |
| `streamMixedAudio` | At each 20 ms pacing opportunity, read one frame from the mixed PCM stream, encode Opus once, and write once to the WebRTC audio track. |
| `close` | Close all connection-scoped resources in lifecycle order. |

Before any RPC, HTTP, Event, packet, or audio loop starts, `PeerConn` atomically ensures its durable Peer generation and publishes the exact connection in `Manager`; therefore an immediate `server.register` cannot precede connection activation. When `server.peer.delete` starts, that exact connection enters retiring and its Manager entry enters deleting before the durable mutation runs. New work, registration, and replacement activation for that public key are rejected, while unrelated Peers are not blocked by the store operation. A successful mutation conditionally detaches the same generation; a failed mutation restores it only when it is still current. The current delete RPC transport remains available until the acknowledgement and EOS write attempt finishes. The terminal action closes the full Giznet connection even when the response or EOS write fails.

`streamMixedAudio` is the sole send-pacing owner for generated audio. When an ordinary Go ticker is late, the sender continues with the next frame without dropping, reordering, or batch-replaying PCM and without creating a provider epoch. Pion owns SSRC, RTP sequence numbers, and timestamps for the live WebRTC track; each 20 ms Opus sample advances the 48 kHz RTP clock by 960 ticks, and a new connection starts an independent RTP timeline. Arrival jitter, adaptive playout delay, packet-loss concealment, and Opus FEC belong to the WebRTC receiver.
