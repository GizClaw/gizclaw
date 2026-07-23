# Stream Events

`Implementation file: peer_stream_event.go`

| Documentation | Features included |
| --- | --- |
| `peer_stream_event.go` | Maintain the connection-scoped Peer event subscriber/broadcast broker; encode and validate `PeerEvent` Protobuf; convert stream payloads to and from GenX chunks; broadcast Agent output and resource invalidations; push valid upstream stream events to the Agent input source. |

This prefix owns the product mapping between the GizClaw Peer event stream and GenX chunks. The underlying stream transport belongs to `pkgs/giznet`; domain state changes remain owned by the service that generated each event.

See the [Events Reference](/references/events) for event types, fields, directions, and BOS/EOS boundaries. See the [Streams Reference](/references/streams) for the relationship among the Event Stream, media, direct packets, and RPC streams. This page records implementation responsibilities only.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `peerStreamEventBroker` | Manage the connection's single event stream subscriber and broadcast product events. |
| `peerAgentOutput` | Consume Agent output, broadcast events, and pass audio to `MixerOutput`. |
| `readPeerStreamEvent` / `writePeerStreamEvent` | Accept only `FrameTypeBinary`, encode/decode `PeerEvent` Protobuf, and validate the `type`/`oneof payload` pair. |
| `peerStreamEventToChunk` | Convert product events into GenX message chunks. |
| `peerStreamEventsFromChunk` | Expand a GenX chunk into one or more product events. |
| `pushAgentChunk` | Push the received event chunk into the Agent input source. |

Downlink audio has no raw Direct Opus branch. `MixerOutput` owns one decoder and PCM track per `(StreamID, canonical MIME)` key. MIME EOS closes only the matching track, while control-only EOS closes every track on that route. Normal EOS uses `CloseWrite` to drain buffered PCM; error EOS uses `CloseWithError` to discard the matching track buffer.
