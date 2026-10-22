# Speed Test

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw)

源码：[rpc_speed.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_speed.go)

实现双向 RPC speed test：校验测试参数、发送和接收指定长度的 binary frames、分别统计
上下行字节与方向耗时，并计算 Mbps。`Duration` 保留整个调用 wall time；
`UpDuration` 和 `DownDuration` 分别从该方向开始传输到包含远端消费完成屏障的结束时点，
`UpMbps` 和 `DownMbps` 只使用对应方向的耗时。双向同时运行时两个方向都包含共享完成屏障；
需要独立测量链路时应分别运行 upload-only 和 download-only。旧调用方未设置方向耗时时，
rate 计算会回退到 `Duration`，保持原有行为。

该能力用于测试 RPC/DataChannel 数据路径，不代表业务吞吐保证。
