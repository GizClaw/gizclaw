# Firmware RPC

`Implementation file: rpc_firmware.go`

A RegistrationToken may bind one Firmware release-line ID to a Peer. The channel is never stored on the token or Peer; the device still chooses `stable`, `beta`, `develop`, or `pending` for each download.

Devices do not list or select Firmware:

- `server.firmware.get` uses an empty request and returns metadata and slots for the caller Peer's bound Firmware.
- `server.firmware.files.download` accepts only `channel` and `path`; the Server resolves the same Peer binding and streams that file.
- A missing binding, missing bound Firmware, or missing artifact returns an explicit not-found error.

Firmware catalog, release lines, and artifact ownership remain in `services/device/firmware` and are managed through the Admin surface.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcFirmwareDownloadService` | The minimum interface used by the Firmware download handler. |
| `handleFirmwareBinDownload` | Parses channel/path, writes metadata, then streams binary frames from the bound Firmware. |
| `writeReaderBinaryFrames` | Split the reader content and write it into RPC binary frames. |
