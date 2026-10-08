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

## 固件 Metadata

Firmware 的可选 `spec.metadata` 与 `spec.slots` 同级，独立于发布渠道。
它是 key 到任意 JSON 值的映射，值可以是对象、数组、字符串、数字、布尔值或 `null`。
版本、URL 和其他字段均由使用方定义，Server 只保存和读取，不解析业务含义。

```yaml
spec:
  slots:
    stable: {}
    beta: {}
    develop: {}
  metadata:
    modem:
      version: vendor-2026.10
      urls:
        - https://firmware.example.com/modem/ap.bin
        - https://firmware.example.com/modem/cp.bin
    label: Devkit
    enabled: true
    optional: null
```

`server.firmware.metadata.get`（138）接受 `{key: "modem"}`，从 caller 已绑定的
Firmware 返回 `{key, value}`。`value` 是紧凑的 UTF-8 JSON 文本；字符串值也保留 JSON
引号，需要按 JSON 解码。存在且值为 `null` 的 key 返回 `value: "null"`，与不存在的 key
不同。key 精确匹配，不解析点号或路径；请求不接受 Firmware ID 或 channel。
未绑定、Firmware 不存在或 key 不存在时返回 `NOT_FOUND`；缺失或非法 key 返回
`INVALID_ARGUMENT`；存储错误返回不携带底层诊断的 `INTERNAL`。

最多配置 64 个 key，每个 key 匹配 `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`。
每个值按持久化时的 JSON 编码计算，最多 65536 个 UTF-8 bytes。
Go 保存原始 JSON 数字，不经过 float64 转换；RPC 返回完整数字文本。
Admin create/put、resource apply/show 和 Peer HTTP `/device/firmware` 保留这些 JSON 值。
PUT/apply 完整替换配置；省略或清空 metadata 会移除已有条目。

Go 使用 `Client.GetFirmwareMetadata` 和原始 `rpcpb.FirmwareMetadataGetRequest` /
`FirmwareMetadataGetResponse`；Flutter 使用 `GizClawClient.getFirmwareMetadata(key)`，
再用 `jsonDecode(response.value)` 解码；JavaScript 使用
`PeerRPCClient.getFirmwareMetadata(key, options?)`，再用 `JSON.parse(response.value)` 解码。
C 使用 `gzc_client_get_firmware_metadata(client, key, timeout_ms, options, &request)`，
沿用 `gzc_client_poll`、`gzc_rpc_request_result`、`gzc_rpc_request_destroy` 的异步生命周期。
Response `value` 是 nanopb callback，调用方安装 decoder 读取 JSON 文本，避免大型静态
value 缓冲区。key 在启动调用中复制，调用方不需要为请求生命周期保留 key 缓冲区。

## 核心结构

| 符号 | 作用 |
| --- | --- |
| `FirmwareGet` | 校验 channel，解析 Peer 绑定并返回该 channel 的 package 配置。 |
| `FirmwarePackage` | Admin 侧 external package contract：SemVer version、HTTPS URL、SHA-256 和 compressed size。 |
