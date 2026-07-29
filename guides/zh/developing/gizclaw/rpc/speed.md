# Speed Test

`实现文件：rpc_speed.go`

实现双向 RPC speed test：校验测试参数、发送和接收指定长度的 binary frames、分别统计
上下行字节与方向耗时，并计算 Mbps。`Duration` 保留整个调用 wall time；
`UpDuration` 和 `DownDuration` 分别从该方向开始传输到包含远端消费完成屏障的结束时点，
`UpMbps` 和 `DownMbps` 只使用对应方向的耗时。双向同时运行时两个方向都包含共享完成屏障；
需要独立测量链路时应分别运行 upload-only 和 download-only。旧调用方未设置方向耗时时，
rate 计算会回退到 `Duration`，保持原有行为。

该能力用于测试 RPC/DataChannel 数据路径，不代表业务吞吐保证。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`SpeedTestResult`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult) | 保存上下行字节、方向耗时与总 wall time。 |
| [`SpeedTestResult.UpMbps`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult.UpMbps) / [`DownMbps`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#SpeedTestResult.DownMbps) | 计算上下行 Mbps。 |
| `callRPCSpeedTest` | Client-side speed test 流程。 |
| `handleSpeedTest` | Server-side speed test streaming handler。 |
| `validateSpeedTestRequest` | 校验上下行长度和测试参数。 |
| `writeBinaryFrames` / `readBinaryFrames` | 写入或读取指定总长度的 binary frames。 |
| `mbps` | 根据字节数和耗时计算 Mbps。 |
