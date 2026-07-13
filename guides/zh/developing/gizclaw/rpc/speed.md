# Speed Test

`实现文件：rpc_speed.go`

实现双向 RPC speed test：校验测试参数、发送和接收指定长度的 binary frames、统计上下行字节与耗时，并计算 Mbps。

该能力用于测试 RPC/DataChannel 数据路径，不代表业务吞吐保证。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`SpeedTestResult`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult) | 保存上下行统计与测试耗时。 |
| [`SpeedTestResult.UpMbps`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult.UpMbps) / [`DownMbps`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult.DownMbps) | 计算上下行 Mbps。 |
| `callRPCSpeedTest` | Client-side speed test 流程。 |
| `handleSpeedTest` | Server-side speed test streaming handler。 |
| `validateSpeedTestRequest` | 校验上下行长度和测试参数。 |
| `writeBinaryFrames` / `readBinaryFrames` | 写入或读取指定总长度的 binary frames。 |
| `mbps` | 根据字节数和耗时计算 Mbps。 |
