# Stream Events

`Implementation file: peer_stream_event.go`

| Documentation | Features included |
| --- | --- |
| `peer_stream_event.go` | Maintain the Peer event subscriber/broadcast broker; convert bidirectionally between `PeerStreamEvent` and GenX message chunks; process text, control, and blob/audio events; broadcast Agent output events and push received events back to the Agent input source. |

This prefix owns the product mapping between the GizClaw Peer event stream and GenX chunks. The underlying stream transport belongs to `pkgs/giznet`; domain state changes remain owned by the service that generated each event.

See the [Events Reference](/references/events) for event types, fields, directions, and BOS/EOS boundaries. See the [Streams Reference](/references/streams) for the relationship among the Event Stream, media, direct packets, and RPC streams. This page records implementation responsibilities only.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `peerStreamEventBroker` | Manage event stream subscribers and broadcast product events. |
| `peerAgentOutput` | Consume Agent output, broadcast events, and pass audio to `MixerOutput`. |
| `readPeerStreamEvent` / `writePeerStreamEvent` | Decode and encode Peer stream events. |
| `peerStreamEventToChunk` | Convert product events into GenX message chunks. |
| `peerStreamEventsFromChunk` | Expand a GenX chunk into one or more product events. |
| `pushAgentChunk` | Push the received event chunk into the Agent input source. |

Downlink audio has no raw Direct Opus branch. `MixerOutput` owns one decoder and PCM track per `(StreamID, canonical MIME)` key. MIME EOS closes only the matching track, while control-only EOS closes every track on that route. Normal EOS uses `CloseWrite` to drain buffered PCM; error EOS uses `CloseWithError` to discard the matching track buffer.
