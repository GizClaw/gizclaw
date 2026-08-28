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

对于已经激活的 physical `edge-node`，`PeerConn` 注册 v2 tunnel namespace，并接受由带
label 的原生 control、packet 与 service DataChannel 聚合出的 logical connection。结果仍是
普通 `giznet.Conn`：key 与 endpoint 来自受信 Edge 的 canonical control label，service
authorization 与 mandatory Event-before-activation 规则都按 logical client 执行。control
或 packet channel 关闭会终止完整 logical Peer，但不关闭 physical Edge 或 sibling session。

正常 Client / Device 连接在产品层 ready 前必须已经具有一条 Opus RTP uplink、
一条 Opus RTP downlink、一条 unordered `maxRetransmits=0` Direct Packet
DataChannel 和一条 reliable ordered `0x20` Event Stream。`pkgs/giznet/gizwebrtc`
负责前三项的 WebRTC mechanics；`PeerConn.serve` 先接受并订阅唯一 Event Stream，
再调用 `activateConn` 发布 Peer。Event 缺失超时或任一必需 transport 关闭都会
关闭完整 connection。额外 Event stream 被关闭，不替换已绑定的实例。

## Service stream 写入流控

JavaScript、Flutter 和 C SDK 对 reliable、ordered service DataChannel 使用每 channel 串行 writer。JavaScript 与 Flutter 的每个原生 DataChannel message 最多承载 16 KiB，面向嵌入式的 C SDK 使用更保守的 4 KiB 上限；writer 到达 high-water 后暂停，收到 buffered-amount-low 通知且队列降到 low-water 后才继续。一次写入成功表示该逻辑消息的全部分片已被本地 WebRTC 发送队列接受，不表示远端已经消费。

JavaScript 与 Flutter 的 high/low water 固定为 1 MiB / 256 KiB。C SDK 默认使用 256 KiB / 64 KiB，嵌入式调用方可以通过 `gzc_client_config_t` 的 `service_write_high_water_bytes` 和 `service_write_low_water_bytes` 调大；自定义值必须满足 high-water 至少 4 KiB 且 low-water 小于 high-water。C 的同步发送只在调用期间借用 caller payload，并使用 `write_timeout_ms` 限制整个逻辑写入；elapsed timeout 读取 platform 的单调 `time_instant_ms`，协议时间戳仍读取 `time_unix_ms`。

Direct packet、Telemetry 和 RTP 不走这套 service stream writer，也不继承这些阈值。

C SDK 保留独立的 `gzc_webrtc_media_vtable_t` 扩展以公开双向 Opus RTP 能力，不改变既有 public struct 的布局。应用在 connect 前注册扩展，之后继续用 `gzc_client_send_packet` / `gzc_client_read_packet` 收发 `GZC_PROTOCOL_OPUS_PACKET`；SDK 对 `0x10` 单独分流，既不写入也不伪装成 packet DataChannel message。WebRTC backend 持有 connection-scoped sendrecv track、RTP header 与时钟，并根据每个 Opus packet 的实际时长推进 48 kHz timestamp。

C SDK 用一次连续 platform allocation 持有固定槽位的 Opus 接收 ring，默认容量为 64 个 packet，在普通 20 ms packet cadence 下覆盖约 1.28 秒。调用方可以在 connect 前用 `gzc_client_set_opus_rx_capacity` 选择其他容量；connect 后容量固定，满队列只覆盖最旧 Opus，不影响 Direct Packet。client close 立即丢弃剩余 ring 内容；保持连接的 conversation cancel 可以在串行 poll-owner 线程调用 `gzc_client_discard_opus_rx`，避免旧音频延后交付。每个槽位最多保存 `GZC_OPUS_MAX_PACKET_SIZE` bytes，接收 callback、入队和 `gzc_client_read_packet_into` 均不为单个 packet 动态分配；后者写入 caller-owned storage，容量不足时返回 `GZC_ERR_BUFFER_TOO_SMALL`、报告所需长度并保留同一 packet 供重试。既有 `gzc_client_read_packet` 继续使用 allocator-owned `gzc_buf_t` 以保持兼容。

C SDK 的并发 unary requester 不新增 connection-scoped transport：每个
`gzc_rpc_request_t` 独占一条动态 RPC service DataChannel，client 只按 channel
identity 做非 owning 分发。单一调用方负责 `gzc_client_poll`，request result 不会
再次进入 poll。client close 会先把所有 pending request 置为 closed 并摘除 channel，
但 request handle 及其响应缓冲区仍由调用方在 allocator 存活期间 destroy。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`PeerConn`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerConn) | 持有 Giznet connection、PeerService、RPC Server、Agent Host、audio mixer 与 connection-scoped services。 |
| [`PeerConn.CreateAudioTrack`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerConn.CreateAudioTrack) | 创建写入当前 Peer audio mixer 的 track。 |
| `serve` | 并行服务 Giznet services、direct packets、Agent output 和 mixed audio。 |
| `serveService` | 接受并分发当前 Peer 打开的 Giznet service stream。 |
| `servePackets` / `serveDirectPackets` | 接收普通与 direct packet，并分发 telemetry/media。 |
| `serveRPC` / `serveEdgeRPC` | 启动 Peer RPC 或 Edge RPC service loop。 |
| `serveEdgeTunnel` / `serveEdgePackets` | 只在 activated Edge 上接受 v2 label、聚合 logical Peer，并路由 shared Opus envelope。 |
| `init` / `initRPC` / `initMixer` / `initAgentHost` / `initPeerGenX` | 组装 connection-scoped runtime dependencies。 |
| `acceptMandatoryEventStream` / `readEventStream` | 在 Peer activation 前有界等待唯一 Event stream，并把事件推入 Agent input；stream 结束会关闭 connection。 |
| `rejectDuplicateEventStreams` | 接受并关闭额外 `0x20`，保留已绑定的 connection owner。 |
| `processTelemetryPackets` / `handleTelemetryPacket` | 解码 telemetry 并同步 Peer status。 |
| `streamMixedAudio` | 在每个 20ms pacing opportunity 从已混合 PCM stream 读取一帧，编码一次 Opus，并写入一次 WebRTC audio track。 |
| `close` | 按 lifecycle 顺序关闭所有 connection-scoped 资源。 |

在启动任何 RPC、HTTP、packet 或 audio loop 前，`PeerConn` 先绑定 Event Stream，
再原子确保 durable Peer generation 并把准确 connection 发布到 `Manager`，因此
不存在“Peer 已 online 但没有 Event transport”的窗口，立即到达的
`server.register` 也不会早于 connection activation。`server.peer.delete` 开始时，准确的 connection 会进入 retiring，其 Manager 条目会在 durable mutation 前进入 deleting。该 public key 的新工作、registration 与 replacement activation 会被拒绝，但 store 操作不会阻塞其他 Peer。mutation 成功后只条件摘除同一 generation；失败时也只在它仍是 current generation 时恢复。当前删除 RPC 的 transport 会保留到 acknowledgement 与 EOS 写入尝试结束；无论 response 或 EOS 写入是否成功，terminal action 都会关闭完整 Giznet connection。

`streamMixedAudio` 是生成音频唯一的发送 pacing owner。普通 Go ticker 迟到时继续读取下一帧，不丢弃、重排或批量补发 PCM，也不创建 provider epoch。Pion 在同一条 WebRTC track 生命周期内维护 SSRC、RTP sequence number 和 timestamp；每个 20ms Opus sample 在 48kHz RTP clock 上推进 960 ticks，新连接建立独立 RTP timeline。到达 jitter、adaptive playout delay、packet-loss concealment 与 Opus FEC 属于 WebRTC receiver。

`PeerConn` 不根据 paced packet 或 mixer read 推导逻辑 BOS/EOS；它只拥有固定的 mixed PCM-to-Opus 下行与实时 pacing。Agent output bridge 在 Mixer 排空后下发聚合后的音频生命周期，因此 transport sequence number、MIME fallback 和 per-source boundary state 都不属于这里。

Agent input runtime 替换时，Realtime Source 把已捕获的旧 user-audio route 交给
`PeerConn`。`PeerConn` 在 `chatroomAccessMu` 内只删除匹配旧 ID 的
`acceptedInputStreams` 和 `acceptedAudio*` 授权；如果 fresh BOS 已经建立了更新 route，
stale callback 不得清除它。释放授权锁后，`PeerConn` 通过现有 event broker 广播精确的
`INPUT_ROUTE_RELOADED` EOS。Event I/O 不在 source 或授权 mutex 下执行；写入失败向
AgentHost 传播，使 reload 不能成功，并由必需 Event transport 生命周期关闭不健康连接。

经 Edge 路由的 connection 由 `PeerConn` 持有 accepted tunnel lifecycle context，并保留 mandatory Event Stream、connection-level first event、Agent input open、first push 和 terminal record。Input event 只有在 authorization 成功后才进入观测；每个 BOS 分配单调递增的 logical turn，后续 input event 通过内部 stream route 关联，input EOS 记录该 turn 的 input terminal，realtime source 第一次成功 push 则证明同一个 turn 已到达 Agent input。Replacement BOS 或成功送达的内部 interrupt 会标记之前的 active turn，但不会改变原有 interruption 行为。Event Stream 关闭时，`PeerConn` 会先为每个仍保留的 incomplete turn 输出一次有界 terminal snapshot，再输出 connection-level terminal，因此后续 zero-output turn 可以被独立查询。

Turn ownership 不根据 output 到达时恰好处于 current 的 turn 推断。Producer response epoch 命名其不可变 owning input route；没有该 provenance 的 response 不归属任何 per-turn record。被替换 turn 及其 input-route 关联会有界保留，直到 owning epoch 完成、route 被 abandon、connection teardown，或 64-turn/64-route 状态上限将其淘汰。Lifecycle 只有观察到显式 `ResponseEpochEnd` 标记才认为 epoch 完成；尚未观察到 route 的空集合及普通 per-MIME EOS 都仍是不完整状态。Producer terminal 在该标记处记账，output 与 turn terminal 则继续等待 Peer broadcast 成功及可能的 audio drain。接受第 65 个 epoch mapping 前，会用 `incomplete/state_limit` 终止最旧 mapping 的真实 owner，并一起释放该 owner 的全部 mapping；释放后迟到的 epoch chunk 保持 unowned，不得重建关联。

Connection 只在面向 Agent 的读取边界包装 realtime input，因此 `agent_transform_started` 证明 transformer 已消费，`agent_input_first_push` 仍只表示 queue 已接受。Producer callback 在 consumer 调度前把 output route 绑定到当前 turn；delivery callback 只在 Peer event 已交付且相关 audio 已 drain 后运行。这些 observer 不等待、不重试、不重排、不复制 payload；每个 boundary 只记录首个 output modality，并为 terminal record 保留有界 modality snapshot。
