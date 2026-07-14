# Connection

`实现文件：peer_conn.go、peer_conn_openai.go`

`peer_conn` 前缀拥有单条 Peer connection 的产品级生命周期。

| 文件 | 包含的功能 |
| --- | --- |
| `peer_conn.go` | `PeerConn` 主生命周期；接受 Giznet service 与 packet；启动普通 RPC 和 Edge RPC；初始化 audio mixer、Agent Host、Peer GenX 与 resource view；处理 event stream、direct packet、telemetry packet 和混音音频输出；统一关闭 connection-scoped 资源。 |
| `peer_conn_openai.go` | 在当前 Peer connection 上提供 OpenAI-compatible HTTP service；组装 Peer resource view 与 ACL authorizer；接入 OpenAI API 和 voice list 等兼容入口。 |

通用 WebRTC、packet transport 和 service stream 属于 `pkgs/giznet`；通用 audio codec 属于 `pkgs/audio`；可持久化 runtime 状态属于 `services/runtime`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`PeerConn`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerConn) | 持有 Giznet connection、PeerService、RPC Server、Agent Host、audio mixer 与 connection-scoped services。 |
| [`PeerConn.CreateAudioTrack`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerConn.CreateAudioTrack) | 创建写入当前 Peer audio mixer 的 track。 |
| `serve` | 并行服务 Giznet services、direct packets、Agent output 和 mixed audio。 |
| `serveService` | 接受并分发当前 Peer 打开的 Giznet service stream。 |
| `servePackets` / `serveDirectPackets` | 接收普通与 direct packet，并分发 telemetry/media。 |
| `serveRPC` / `serveEdgeRPC` | 启动 Peer RPC 或 Edge RPC service loop。 |
| `init` / `initRPC` / `initMixer` / `initAgentHost` / `initPeerGenX` | 组装 connection-scoped runtime dependencies。 |
| `serveEvents` / `handleEventStream` | 接受 event stream 并推入 Agent input。 |
| `processTelemetryPackets` / `handleTelemetryPacket` | 解码 telemetry 并同步 Peer status。 |
| `streamMixedAudio` | 将 mixer 输出编码并发送给 Peer。 |
| `close` | 按 lifecycle 顺序关闭所有 connection-scoped 资源。 |
