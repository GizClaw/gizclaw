# Speed Test

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw)

Source：[rpc_speed.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_speed.go)

Implements the bidirectional RPC speed test: validates parameters, sends and
receives the requested binary-frame lengths, records each direction
independently, and calculates Mbps. `Duration` remains the whole-call wall time.
`UpDuration` and `DownDuration` run from the start to completion of their own
direction, including the remote-consumption completion barrier, and `UpMbps`
and `DownMbps` use only the matching duration. In a bidirectional run both
durations include the shared completion barrier; use separate upload-only and
download-only runs when measuring each path independently. Results constructed
by older callers without a direction duration retain the previous rate
calculation by falling back to `Duration`.

This capability is used to test the RPC/DataChannel data path and does not represent a guarantee of business throughput.
