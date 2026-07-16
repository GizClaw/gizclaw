# Speed Test

`Implementation file: rpc_speed.go`

Implement bidirectional RPC speed test: verify test parameters, send and receive binary frames of specified length, count uplink and downlink bytes and time consumption, and calculate Mbps.

This capability is used to test the RPC/DataChannel data path and does not represent a guarantee of business throughput.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`SpeedTestResult`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult) | Stores uplink and downlink statistics and test time consumption. |
| [`SpeedTestResult.UpMbps`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult.UpMbps) / [`DownMbps`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult.DownMbps) | Calculate upstream and downstream Mbps. |
| `callRPCSpeedTest` | Client-side speed test process. |
| `handleSpeedTest` | Server-side speed test streaming handler. |
| `validateSpeedTestRequest` | Verify the uplink and downlink length and test parameters. |
| `writeBinaryFrames` / `readBinaryFrames` | Write or read binary frames of the specified total length. |
| `mbps` | Calculate Mbps based on the number of bytes and time taken. |
