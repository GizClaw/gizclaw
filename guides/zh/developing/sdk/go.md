# Go SDK <Badge type="warning" text="WIP" />

> 本页目前只说明 SDK 的定位和范围，公开 client、连接、RPC、stream 与 telemetry 模块仍待逐项展开。

`sdk/go/gizcli` 提供 Go 调用方使用的 GizClaw client surface，包括连接、安全策略、资源、Peer stream、RPC、Telemetry 与 WebRTC 接入。

Go SDK 是 client-facing boundary，不拥有 server domain behavior。API 和 RPC method 来自 [API Design](../api/overview)；修改 contract 时必须同步生成 surface、SDK 实现和测试。

调用方删除当前 Peer 时使用 `Client.DeletePeer`。成功 response 会终止当前 Peer connection，调用方必须重新连接后才能继续发起工作。

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli)
