# Go SDK <Badge type="warning" text="WIP" />

本页将说明 Go SDK 的安装、client 初始化、认证、连接、API/RPC 调用和错误处理。

## 握手准入凭证

`gizwebrtc.DialConfig.Credential` 直接接收 `*giznetpb.AdmissionCredential`。调用方可设置
`Version`、`Type`、`Value`；GizClaw 的 `gizcli.RegistrationTokenCredential(token)` helper
构造 version 1、type `registration_token` 的结构。SDK 执行 protobuf 编码，无凭证使用 nil。
编码和边界见 [Giznet](../../developing/giznet#signaling-准入凭证)。握手不绑定资源，连接后
仍需 `server.register`；准入条件见 [Security Policy](../../developing/gizclaw/server/security-policy)。
