# Firmware Download

`Implementation file: rpc_firmware.go`

Process Firmware binary download streaming RPC: parse the request, call the firmware download service, return metadata first, and then write the artifact reader into consecutive binary frames.

Firmware catalog, licensing and artifact ownership belong to `services/device/firmware`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcFirmwareDownloadService` | The minimum domain interface that the Firmware download handler depends on. |
| `handleFirmwareBinDownload` | Verify request, obtain artifact, and write metadata and binary frames. |
| `writeReaderBinaryFrames` | Split the reader content and write it into RPC binary frames. |
