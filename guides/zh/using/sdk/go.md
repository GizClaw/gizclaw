# Go SDK

Go API Reference：[gizcli](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli)。

## 接收正在播放的 Opus 流

`gizcli.Client.OpenPeerStream` 和 `DialPeerEventStream` 的新订阅会先收到当前仍在播放的音频 BOS 元数据，然后接收实时事件和 Opus 包。因此，连续的 Giztest `listen` 窗口可以在播报中途接续接收，而不需要重新启动播报。

连接只保留一个当前下行音频起始事件，不回放历史音频或文字。音频 EOS 清除对应路由；同 ID 的文字 EOS、输入侧 `user` BOS/EOS 和旧音频路由的延迟 EOS 不清除新路由。订阅的初始元数据与实时事件按顺序交付，连接结束后不保留路由。

## PeerStream 音频收尾

`Client.OpenPeerStream` 将 Event Stream 与 Opus RTP 合并为逻辑音频流。在交付 audio EOS 前，
它先交付当前已排队的音频；下行 audio EOS 后、下一次 audio BOS 前收到的包被忽略，不交付为无归属
音频，也不追加给下一轮。BOS 恢复接收前先忽略结束期间已排队的包；text event 与连接生命周期继续独立运行。

`PeerStream.IgnoredPostEOSAudioPackets()` 可并发读取该 stream 生命周期内累计忽略的包数。
EOS 表示生产结束，接收端不会为等尾包添加固定延迟。这是逻辑流消费策略，不改变
WebRTC/RTP、Server、Edge 或 wire Schema；使用原始 media callback 的客户端自行管理收尾。

输入侧 `user` EOS（包括 `INPUT_ROUTE_RELOADED`）继续作为控制事件交付，不会关闭助手下行音频。

## 握手准入凭证

`gizwebrtc.DialConfig.Credential` 直接接收 `*giznetpb.AdmissionCredential`。调用方可设置
`Version`、`Type`、`Value`；GizClaw 的 `gizcli.RegistrationTokenCredential(token)` helper
构造 version 1、type `gizclaw.com/registration_token` 的结构。SDK 执行 protobuf 编码，无凭证使用 nil。
编码和边界见 [Giznet](../../developing/giznet#signaling-准入凭证)。握手不绑定资源，连接后
仍需 `server.register`；准入条件见 [Security Policy](../../developing/gizclaw/server/security-policy)。

`RegistrationTokenCredential` 返回 `(credential, error)`；先处理错误，再把结构传给 Dial。`gizcli.RegistrationTokenCredentialType` 是内置 type 的导出常量。512 UTF-8 字节的 value 可用，513 字节在 helper 构造时失败。

## MHS v0 HWD

通过 `gizcli.Client.HandleDeviceControl` 安装 `DeviceControlHandlers.ReadMhsHwd` 和 `WriteMhsHwd`。handler 收到 `rpcpb.ClientMhsV0ReadRequest` 或 `ClientMhsV0WriteRequest`，按 `ClientHwd` 用 `rpcapi.ClientHwdReadResponseMessage` / `ClientHwdWriteRequestFromBytes` 等注册表函数处理对应 protobuf。读写每次只针对一个 `id`；wifi、ble、modem、battery、mic 只读，display、led、speaker 可写。不存在的物理实例可返回 `ErrDeviceResourceNotFound`。

RuntimeProfile manifest 离线可读，只声明实例 ID 和 HWD 类型；设备端驱动执行安全限制并在写入响应中返回实际生效值。完整契约见 [Public API](/zh/developing/api/http/public) 与 [provider contract](/zh/developing/api/proto/rpc/client-provided-to-server)。
