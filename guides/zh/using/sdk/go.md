# Go SDK <Badge type="warning" text="WIP" />

本页将说明 Go SDK 的安装、client 初始化、认证、连接、API/RPC 调用和错误处理。

## 握手准入凭证

`gizwebrtc.DialConfig.Credential` 直接接收 `*giznetpb.AdmissionCredential`。调用方可设置
`Version`、`Type`、`Value`；GizClaw 的 `gizcli.RegistrationTokenCredential(token)` helper
构造 version 1、type `gizclaw.com/registration_token` 的结构。SDK 执行 protobuf 编码，无凭证使用 nil。
编码和边界见 [Giznet](../../developing/giznet#signaling-准入凭证)。握手不绑定资源，连接后
仍需 `server.register`；准入条件见 [Security Policy](../../developing/gizclaw/server/security-policy)。

`RegistrationTokenCredential` 返回 `(credential, error)`；先处理错误，再把结构传给 Dial。`gizcli.RegistrationTokenCredentialType` 是内置 type 的导出常量。512 UTF-8 字节的 value 可用，513 字节在 helper 构造时失败。

## MHS v0 硬件状态

通过 `gizcli.Client.HandleDeviceControl` 安装 `DeviceControlHandlers.ReadMhsStates` 和 `WriteMhsStates`，参数/响应直接使用 `rpcpb.ClientMhsV0*`。handler 返回 `ErrDeviceResourceNotFound` 表示硬件未实现该 key；返回 `rpcapi.Error{Code: rpcapi.StatusCodeFailedPrecondition}` 表示当前不能写入。

这是 GizClaw 自有的 MHS-inspired 预标准 v0，不声称官方兼容。manifest 离线可读，每次读写最多 32 个唯一 key；写入必须整批验证且驱动执行安全限制。完整错误与边界见 [Public API](/zh/developing/api/http/public) 和 [provider contract](/zh/developing/api/proto/rpc/client-provided-to-server)。
