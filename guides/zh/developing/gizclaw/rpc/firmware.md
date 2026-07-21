# Firmware RPC

`实现文件：rpc_firmware.go`

Peer 通过 RegistrationToken 可选绑定一个 Firmware release-line ID。绑定写入 Peer；channel 不写入 Token 或 Peer，仍由设备在每次下载时选择 `stable`、`beta`、`develop` 或 `pending`。

设备不列举或选择 Firmware：

- `server.firmware.get` 使用空 request，Server 根据 caller Peer 的 `firmware_id` 返回当前 Firmware metadata 与 slots。
- `server.firmware.files.download` 只接收 `channel` 和 `path`，Server 使用同一个 Peer 绑定解析 Firmware 并流式返回文件。
- Peer 未绑定 Firmware、绑定目标不存在或 artifact 不存在时返回明确的 not-found error。

Firmware catalog、release-line 和 artifact ownership 仍属于 `services/device/firmware`，由 Admin surface 管理。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcFirmwareDownloadService` | Firmware download handler 依赖的最小 interface。 |
| `handleFirmwareBinDownload` | 解析 channel/path，写入 metadata，再把绑定 Firmware 的文件写成 binary frames。 |
| `writeReaderBinaryFrames` | 将 reader 内容切分并写成 RPC binary frames。 |
