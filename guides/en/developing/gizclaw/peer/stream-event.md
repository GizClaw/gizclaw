# Stream Events

`Implementation file: peer_stream_event.go`

| Documentation | Features included |
| --- | --- |
| `peer_stream_event.go` | Maintain Peer event subscriber/broadcast broker; bidirectionally convert between `PeerStreamEvent` and GenX message chunk; process text, control, blob/audio events; encode Agent output into event stream or raw Opus direct packet; control Opus sending rhythm and push received events back to Agent input source. |

This prefix holds the product mapping for the GizClaw Peer event stream. The underlying stream transport belongs to `pkgs/giznet`; domain state changes are still owned by the service that generated the event.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `peerStreamEventBroker` | Manage event stream subscribers and broadcast product events. |
| `peerAgentOutput` | Consume Agent output, converting it into events or raw Opus packets. |
| `peerOpusPacer` | Controls the sending rhythm of continuous Opus frames. |
| `readPeerStreamEvent` / `writePeerStreamEvent` | Decode and encode Peer stream events. |
| `peerStreamEventToChunk` | Convert product events into GenX message chunks. |
| `peerStreamEventsFromChunk` | Expand a GenX chunk into one or more product events. |
| `pushAgentChunk` | Push the received event chunk into the Agent input source. |
