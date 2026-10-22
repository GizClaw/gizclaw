# Realtime Source

Source：[peer_realtime_source.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_realtime_source.go)

| Documentation | Features included |
| --- | --- |
| `peer_realtime_source.go` | Implement Peer realtime input source; open and close GenX stream, push message chunk, and bind stable stream ID to continuous audio chunk. |

This is responsible for converting connection-scoped input into a realtime source that can be consumed by the Agent runtime. It does not have a common GenX stream contract or Agent instance life cycle. `PeerConn` and AgentHost own recovery: a chunk whose observed runtime revision changed during a workspace transition is dropped rather than retried into a new source.
