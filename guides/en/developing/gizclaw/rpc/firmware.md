# Firmware Download

`Implementation file: rpc_firmware.go`

Retains Firmware binary download streaming RPC parsing and framing compatibility. Firmware is no longer projected to peers by RegistrationToken or RuntimeProfile, so the current download returns not found and never opens an Admin Firmware artifact.

Firmware catalog, authorization, and artifact ownership remain in `services/device/firmware` and are managed only through the Admin surface.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcFirmwareDownloadService` | The minimum interface used by the Firmware download compatibility handler. |
| `handleFirmwareBinDownload` | Validates the request and returns the peer-resource not-found error; metadata and binary frames are written only if a service returns a reader. |
| `writeReaderBinaryFrames` | Split the reader content and write it into RPC binary frames. |
