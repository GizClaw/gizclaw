# Speed Test

`Implementation file: rpc_speed.go`

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

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`SpeedTestResult`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult) | Stores direction bytes, direction durations, and total wall time. |
| [`SpeedTestResult.UpMbps`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult.UpMbps) / [`DownMbps`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult.DownMbps) | Calculate upstream and downstream Mbps. |
| `callRPCSpeedTest` | Client-side speed test process. |
| `handleSpeedTest` | Server-side speed test streaming handler. |
| `validateSpeedTestRequest` | Verify the uplink and downlink length and test parameters. |
| `writeBinaryFrames` / `readBinaryFrames` | Write or read binary frames of the specified total length. |
| `mbps` | Calculate Mbps based on the number of bytes and time taken. |
