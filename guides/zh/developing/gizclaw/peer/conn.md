# Connection

`实现文件：peer_conn.go、peer_conn_openai.go`

`peer_conn` 前缀拥有单条 Peer connection 的产品级生命周期。

| 文件 | 包含的功能 |
| --- | --- |
| `peer_conn.go` | `PeerConn` 主生命周期；接受 Giznet service 与 packet；启动普通 RPC 和 Edge RPC；初始化 audio mixer、Agent Host、Peer GenX 与 resource view；处理 event stream、direct packet、telemetry packet 和混音音频输出；统一关闭 connection-scoped 资源。 |
| `peer_conn_openai.go` | 在当前 Peer connection 上提供 OpenAI-compatible HTTP service；组装 RuntimeProfile 与 owner resource view；接入 OpenAI API 和 voice list 等兼容入口。 |

通用 WebRTC、packet transport 和 service stream 属于 `pkgs/giznet`；通用 audio codec 属于 `pkgs/audio`；可持久化 runtime 状态属于 `services/runtime`。

## 传输 contract

Audio、direct packet、Peer Event Stream、RPC/HTTP service stream 的方向、可靠性、service ID、framing 与生命周期统一由 [Streams Reference](/references/streams) 定义；Event wire type 与字段统一由 [Events Reference](/references/events) 定义。本页只说明 `PeerConn` 如何实现这些 contract，不再复制协议表格。

## Service stream 写入流控

JavaScript、Flutter 和 C SDK 对 reliable、ordered service DataChannel 使用每 channel 串行 writer。每个原生 DataChannel message 最多承载 1400 bytes；writer 到达 high-water 后暂停，收到 buffered-amount-low 通知且队列降到 low-water 后才继续。一次写入成功表示该逻辑消息的全部分片已被本地 WebRTC 发送队列接受，不表示远端已经消费。

JavaScript 与 Flutter 的 high/low water 固定为 1 MiB / 256 KiB。C API v2 默认使用 256 KiB / 64 KiB，嵌入式调用方可以通过 `gzc_client_config_t` 的 `service_write_high_water_bytes` 和 `service_write_low_water_bytes` 调大；自定义值必须满足 high-water 至少 1400 bytes 且 low-water 小于 high-water。C 的同步发送只在调用期间借用 caller payload，并使用 `write_timeout_ms` 限制整个逻辑写入；elapsed timeout 读取 platform 的单调 `time_instant_ms`，协议时间戳仍读取 `time_unix_ms`。

Direct packet、Telemetry 和 RTP 不走这套 service stream writer，也不继承这些阈值。

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
| `streamMixedAudio` | 在每个 20ms pacing opportunity 从已混合 PCM stream 读取一帧，编码一次 Opus，并写入一次 WebRTC audio track。 |
| `close` | 按 lifecycle 顺序关闭所有 connection-scoped 资源。 |

在启动任何 RPC、HTTP、Event、packet 或 audio loop 前，`PeerConn` 会原子确保 durable Peer generation 并把准确 connection 发布到 `Manager`，因此立即到达的 `server.register` 不会早于 connection activation。`server.peer.delete` 开始时，准确的 connection 会进入 retiring，其 Manager 条目会在 durable mutation 前进入 deleting。该 public key 的新工作、registration 与 replacement activation 会被拒绝，但 store 操作不会阻塞其他 Peer。mutation 成功后只条件摘除同一 generation；失败时也只在它仍是 current generation 时恢复。当前删除 RPC 的 transport 会保留到 acknowledgement 与 EOS 写入尝试结束；无论 response 或 EOS 写入是否成功，terminal action 都会关闭完整 Giznet connection。

`streamMixedAudio` 是生成音频唯一的发送 pacing owner。普通 Go ticker 迟到时继续读取下一帧，不丢弃、重排或批量补发 PCM，也不创建 provider epoch。Pion 在同一条 WebRTC track 生命周期内维护 SSRC、RTP sequence number 和 timestamp；每个 20ms Opus sample 在 48kHz RTP clock 上推进 960 ticks，新连接建立独立 RTP timeline。到达 jitter、adaptive playout delay、packet-loss concealment 与 Opus FEC 属于 WebRTC receiver。
