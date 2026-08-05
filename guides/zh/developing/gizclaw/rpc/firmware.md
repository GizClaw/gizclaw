# Firmware RPC

`实现文件：services/runtime/peerresource/firmware.go`

RegistrationToken 可以给 Peer 绑定一个 canonical Firmware ID。设备不列举或
选择 Firmware release line；它通过 `server.firmware.get` 从已绑定的 release
line 请求一个具体 channel。

request 只有 `channel`，可选值为 `stable`、`beta`、`develop` 或 `pending`。
response 包含：

- peer-visible firmware name 和所请求的 channel；
- 可选的 channel description；
- `.tar.zlib` package（一个 tar archive 压缩为单个 zlib stream）的绝对 HTTPS URL；
- 精确对应压缩 package bytes 的 SHA-256 和 byte size。

Peer 可见的 name、description 和 URL 分别最多为 256、1024 和 2048 个 UTF-8
bytes。Package size 必须为正数且不超过 `9007199254740991`，确保 JavaScript SDK
也能精确表示 byte count。

Peer 自己直接下载 URL，并校验压缩后的 bytes。GizClaw 不获取、不解压、不代理、
不上传，也不通过 RPC stream 传输 firmware package。Peer 未绑定 Firmware、绑定目标
不存在、channel 没有 package 或 channel 非法时，返回明确的 RPC error。

Firmware catalog 和 release-channel ownership 仍属于
`services/device/firmware`，由 Admin surface 管理。

## 核心结构

| 符号 | 作用 |
| --- | --- |
| `FirmwareGet` | 校验 channel，解析 Peer 绑定并返回该 channel 的 package 配置。 |
| `FirmwarePackage` | Admin 侧 external package contract：HTTPS URL、SHA-256 和 compressed size。 |
