# Firmware Download

`实现文件：rpc_firmware.go`

保留 Firmware binary download streaming RPC 的解析和 framing compatibility。Firmware 不再由 RegistrationToken 或 RuntimeProfile 投影给 peer，因此当前 download 返回 not found，不会打开 Admin Firmware artifact。

Firmware catalog、授权和 artifact ownership 仍属于 `services/device/firmware`，只通过 Admin surface 管理。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcFirmwareDownloadService` | Firmware download compatibility handler 依赖的最小 interface。 |
| `handleFirmwareBinDownload` | 验证 request 并稳定返回 peer resource 层的 not-found error；只有 service 返回 reader 时才写 metadata 与 binary frames。 |
| `writeReaderBinaryFrames` | 将 reader 内容切分并写成 RPC binary frames。 |
