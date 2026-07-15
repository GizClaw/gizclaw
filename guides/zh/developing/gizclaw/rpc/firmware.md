# Firmware Download

`实现文件：rpc_firmware.go`

处理 Firmware binary download streaming RPC：解析请求、调用 firmware download service、先返回 metadata，再将 artifact reader 写成连续 binary frames。

Firmware catalog、授权和 artifact ownership 属于 `services/device/firmware`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcFirmwareDownloadService` | Firmware download handler 依赖的最小领域 interface。 |
| `handleFirmwareBinDownload` | 验证 request、取得 artifact，并写入 metadata 与 binary frames。 |
| `writeReaderBinaryFrames` | 将 reader 内容切分并写成 RPC binary frames。 |
