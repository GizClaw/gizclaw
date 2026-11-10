# Go SDK

`sdk/go/gizcli` 提供 Go 调用方使用的 GizClaw client surface，包括连接、安全策略、资源、Peer stream、RPC、Telemetry 与 WebRTC 接入。

Go SDK 是 client-facing boundary，不拥有 server domain behavior。API 和 RPC method 来自 [API Design](../api/overview)；修改 contract 时必须同步生成 surface、SDK 实现和测试。

调用方删除当前 Peer 时使用 `Client.DeletePeer`。成功 response 会终止当前 Peer connection，调用方必须重新连接后才能继续发起工作。

## Context 连接与 Admin Resource

两个子 package 为 CLI 与 Terraform provider 提供共享的纯 Go 能力，在 `CGO_ENABLED=0` 下可为 darwin/linux 的 amd64/arm64 构建，不依赖 `cmd/...`、嵌入的 Console 或 native model runtime。

- `sdk/go/gizcli/contextconn`：`ConfigDir` 返回 CLI context 根目录；`LoadContext` 读取 `<ConfigDir>/<context>/config.yaml`（名称为空时读取 current context），并用 `Options.Endpoint` 替换 `server.endpoint`，覆盖值必须是 `https://host[:port]`；`Dial` 获取 server-info 并准备 WebRTC client；`Connect` 连接、启动 client-side Peer service 并等待 Peer HTTP 可用；就绪探测共享 5 秒预算，并服从调用方更早的 deadline。Serve 退出会取消进行中的探测，失败时关闭 client。传入的 `context.Context` 可取消 server-info 请求、WebRTC dial 与就绪等待。调用方拥有返回的 client 并负责 `Close`。
- `sdk/go/gizcli/adminresource`：`Client` 封装 Admin HTTP 的 `ApplyResource`、`GetResource` 与 `DeleteResource`；非成功 response 返回 `*ResponseError`，文本保持 `CODE: message`，`IsNotFound` 只识别 `<SCOPE>_NOT_FOUND` 错误码。`GetResources` 在一条连接上最多并发 8 个读取，并按输入顺序返回每个引用的结果。`PrepareManifest`、`DecodeManifest` 与 `FormatForPath` 实现 `gizclaw admin apply|validate` 的 JSON/YAML、`${VAR}` 展开与 `<Kind>Resource` 别名规则。

[contextconn API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn) · [adminresource API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli)

## 初始音频 BOS 与 RTP

Peer Event Stream 和固定 Opus RTP 轨是独立通道，发送 BOS 成功不代表接收端已经处理
BOS。`PeerStream` 在首次音频 BOS 绑定之前暂存 RTP payload，先向调用方输出 BOS，
再按接收顺序输出带该 StreamID 和 label 的音频。文字事件不触发音频暂存释放。
暂存上限为 64 包或 128 KiB；从收到首包起等待 BOS 最多 1 秒。超限或超时会
释放暂存，并持续丢弃未绑定音频直到首次 BOS 到达，不终止 PeerStream，也不阻塞
文字事件。BOS 到达后正常接收新音频，已丢弃的包不会重放；关闭时同样释放暂存。
这可能丢失超过重排窗口的有效首音频，但不会让找不到 BOS 的孤立包终止整个对话。
在同一连接重新打开 PeerStream 时，旧回复的迟到 RTP 也可能进入这个初始窗口。

这一行为不改变 wire Schema、设备固件或其他语言 SDK，也不增加下行 ACK。RTP
payload 没有逻辑 StreamID，无法据此区分上一 epoch 的迟到包和下一 epoch 的首包；
因此暂存仅用于首次音频绑定，不会将 EOS 后的裸包重归属给下一 BOS。此类包仍由
调用方的边界检查报告；现有 EOS drain 和音频完整性断言保持不变。

## Recovered media output

On RTP connections with native Opus support, `PeerStream` outputs signed
little-endian PCM16 at 16 kHz mono, including PCM BOS/EOS boundaries. The connection's
`giznet/opusmedia` receiver preserves RTP timing, reorders within the configured
[`rtpstream` bounds](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/audio/rtpstream#Config),
and conceals known missing intervals with its continuous Opus decoder.
`PeerStream` only binds these recovered frames to the logical route.

`genx.Blob.OriginalAudio` carries a real received Opus packet when available;
`ConcealedAudio` marks synthesized PCM and has no original encoded packet.
Both fields are process-local and excluded from JSON. Recorders can retain real
original audio without storing synthetic Opus. The physical Event reader drains
its own connection's media receiver once before publishing the active audio EOS
to local subscribers. Each subscriber drains only its own queue; a delayed or
closed subscription cannot reset newer media or a replacement connection. Late
audio remains subject to the existing EOS guard, and EOS does not read future
packets during a grace window.

On runtimes without native Opus, including `CGO_ENABLED=0`, RTP reception uses
bounded encoded reordering and delivers real Opus packets with Opus BOS/EOS.
It does not synthesize encoded packets for missing intervals or provide decoded
PCM/PLC. The output mode is chosen for the physical session before event delivery
and stays fixed through every EOS. Encoded packet subscribers and reliable
control remain available. Legacy payload-only sessions retain raw Opus controls
and data. Within a media receiver, untimed Opus follows the agreed output mode
without a jitter wait: decoded PCM in PCM mode, real originals in encoded mode.
This also keeps controls and payloads consistent for custom timed-capable readers
that supply an untimed packet. Native RTP and tunnel sessions still enforce their
transport timing declarations.

With native Opus support, `PeerStream.Push` also accepts PCM16 little-endian at
16 kHz mono. Its media sender encodes the whole
PCM epoch through one Opus encoder, including concealed intervals, and flushes
its last partial 20 ms codec frame before sending EOS. It never mixes encoded
originals into that stream. Input format cannot change within one epoch. This Go
contract does not assert PLC support in external firmware or native C playback.

## Peer HTTP 状态同步

OpenAPI 生成的 `pkgs/gizclaw/api/peerhttp.Client.SyncPeer` 接受 `SyncPeerParams.Timestamp`，返回可读取 SSE 的 `*http.Response`。请求通过 API Key 绑定 owner；调用方关闭 response body，并用 context 取消请求。`SyncEvent` 是同一 Schema 生成的事件联合类型。暂存每轮状态，在 done 时一起提交状态与服务端时间戳；不完整响应应丢弃并以原时间戳重试。具体事件与范围见 [Public API](../api/http/public#peer-状态同步)。
