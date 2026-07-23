# Realtime Source

`Implementation file: peer_realtime_source.go`

| Documentation | Features included |
| --- | --- |
| `peer_realtime_source.go` | Implement Peer realtime input source; open and close GenX stream, push message chunk, and bind stable stream ID to continuous audio chunk. |

This is responsible for converting connection-scoped input into a realtime source that can be consumed by the Agent runtime. It does not have a common GenX stream contract or Agent instance life cycle. `PeerConn` and AgentHost own recovery: a chunk whose observed runtime revision changed during a workspace transition is dropped rather than retried into a new source.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `peerRealtimeSource` | Holds the current GenX input stream and audio stream ID status. |
| `newPeerRealtimeSource` | Create Peer realtime source. |
| `OpenAgentInput` | Open the input stream for Agent Host consumption. |
| `Push` | Push the Peer message chunk into the current input stream. |
| `bindAudioStreamID` | Bind stable stream ID to continuous audio chunks. |
| `Close` | Close the source and underlying stream. |
