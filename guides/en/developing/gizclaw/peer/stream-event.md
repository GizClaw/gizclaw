# Stream Events

`Implementation file: peer_stream_event.go`

| Documentation | Features included |
| --- | --- |
| `peer_stream_event.go` | Maintain the Peer event subscriber/broadcast broker; convert bidirectionally between `PeerStreamEvent` and GenX message chunks; process text, control, and blob/audio events; broadcast Agent output events and push received events back to the Agent input source. |

This prefix holds the product mapping for the GizClaw Peer event stream. The underlying stream transport belongs to `pkgs/giznet`; domain state changes are still owned by the service that generated the event.

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
