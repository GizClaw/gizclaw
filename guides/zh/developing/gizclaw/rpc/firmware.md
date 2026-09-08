# Firmware RPC

`实现文件：services/runtime/peerresource/firmware.go`

RegistrationToken 可以给 Peer 绑定一个 canonical Firmware ID。设备不列举或
选择 Firmware resource；它通过 `server.firmware.get` 从已绑定的 resource 请求一个具体 channel。

Admin create/apply 的 immutable ID 由 caller 提供。Firmware 没有独立的 `name`
或 `spec.name`；Firmware identity 只用于 Admin binding，不通过 Peer RPC 暴露。

request 只有 `channel`，可选值为 `stable`、`beta` 或 `develop`。
response 包含：

- 所请求的 channel；
- 可选的 channel description；
- `.tar.zlib` package（一个 tar archive 压缩为单个 zlib stream）的绝对 HTTPS URL；
- 精确对应压缩 package bytes 的 SHA-256 和 byte size。

Description 和 URL 分别最多为 1024 和 2048 个 UTF-8 bytes。Package size 必须为正数且不超过 `9007199254740991`，确保 JavaScript SDK
也能精确表示 byte count。

Peer 自己直接下载 URL，并校验压缩后的 bytes。GizClaw 不获取、不解压、不代理、
不上传，也不通过 RPC stream 传输 firmware package。Peer 未绑定 Firmware、绑定目标
不存在、channel 没有 package 或 channel 非法时，返回明确的 RPC error。

Firmware catalog 和声明式 channel ownership 仍属于
`services/device/firmware`，由 Admin surface 管理。

Response 的 `version` 是可选字段，来自所选 channel 的 `package.version`；存在时为最多 128 个 ASCII 字符的严格 SemVer 2.0.0。没有版本的已有包仍可用，服务端省略该字段，不推断版本。Protobuf 字段 6 使用 `optional string`：Go 以可空指针表示，JavaScript 缺失属性读作 `undefined`，Dart 用 `hasVersion()` 判断，C 用 `has_version` 判断并保留 129 bytes 字符串空间（含 NUL）。版本不替代下载完整性校验和 OTA 请求的 SHA-256 guard。

## 核心结构

| 符号 | 作用 |
| --- | --- |
| `FirmwareGet` | 校验 channel，解析 Peer 绑定并返回该 channel 的 package 配置。 |
| `FirmwarePackage` | Admin 侧 external package contract：SemVer version、HTTPS URL、SHA-256 和 compressed size。 |
