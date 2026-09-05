# Client Provided to Server

This set of capabilities is implemented by Client/Device and called by Server on Peer connection. Server uses it to read the device's own information or request the device to perform local capabilities.

The [RPC API Reference](/references/rpc) is the single list of exact method IDs, names, and purposes. This page only explains the `client.*` provider direction and ownership.

## Calling relationship

```mermaid
sequenceDiagram
    participant Server
    participant Client
    Server->>Client: client.* request
    Client->>Client: Read device state or invoke local tool
    Client-->>Server: typed response / RPC error
```

A Client provider can only return data that is owned or executable by the Client. Server resource-access decisions, cross-peer lookup, and persistence management cannot be implemented as `client.*`.

Go Client's provider dispatch is located in the `sdk/go/gizcli` RPC Client implementation. A C Client registers the same provider direction through `gzc_client_config_t.rpc_provider`; the callback supplies borrowed Protobuf response bytes or a stable RPC error before returning. The server side calls these methods through the online Peer connection.

## Device control providers

`client.device.status.get` (100), `client.device.volume.set` (101), `client.device.sound.play` (102), `client.device.reboot` (103), `client.wifi.status.get` (104), `client.wifi.saved.list` (105), `client.wifi.saved.forget` (106), `client.wifi.scan` (108), `client.wifi.connect` (109), and `client.firmware.update` (111) are implemented by the device `rpc_provider`; the Server calls them while serving Public HTTP `/gizclaw/v1/device*` control requests. Controls use a 5-second timeout except scan, which uses the requested 1–15 second bound. Provider responsibilities:

- `volume.set` applies an absolute `level` (0–100) and `muted` and returns the complete post-change `PeerStatus`; `status.get` returns the current `PeerStatus`. Repeating a call with equal input yields the same result.
- `sound.play` takes a device-defined `sound` string (at most 32 UTF‑8 bytes) that the device validates; unknown values answer `INVALID_PARAMS`. `duration_ms` is optional.
- `reboot` must send its response before rebooting; `delay_ms` is optional.
- `firmware.update` must send its response before running the OTA. `channel` names the channel to install and defaults to the channel the device already uses; `sha256` is the digest the caller saw, and the device answers `INVALID_PARAMS` when it does not match the package the device resolves. The device downloads, verifies, writes, and restarts on its own, and answers success outright when it already runs the target package. It reports the package it currently runs as `PeerStatus.firmware_sha256`, which `status.get` and `volume.set` responses write back to the Server.
- `wifi.status.get` returns `WifiStatus { connected, ssid, rssi_dbm, ip, bssid }`; `wifi.saved.list` returns the saved network `ssid`s; `wifi.saved.forget` answers `NOT_FOUND` for an unknown `ssid`, including a network that was already forgotten. `ssid` is at most 32 UTF‑8 bytes and nanopb bounds the field.
- `wifi.scan` returns `WifiScanResult` entries within `timeout_ms`, sorted by descending `rssi_dbm` and deduplicated by SSID to the strongest entry. `security` is a lowercase device-reported identifier that the Server does not enumerate.
- `wifi.connect` accepts an open network or an 8–63 byte PSK. The device must return `ClientWifiConnectResponse` before disconnecting and switching networks, and fall back to the old network on failure. It must never persist or log the passphrase or include it in errors.
- A device returns only results it can execute itself: invalid parameters answer `INVALID_PARAMS`, unimplemented methods answer `METHOD_NOT_FOUND`, and other failures answer `INTERNAL_ERROR` with a short message. The Server maps these to `400 DEVICE_REJECTED`, `501 DEVICE_UNSUPPORTED`, and a redacted `502 DEVICE_ERROR`.

The C SDK `inbound_is_client_method` accepts these device control methods and dispatches them to `gzc_client_config_t.rpc_provider`; a missing provider or an unhandled method answers `METHOD_NOT_FOUND`. The Go SDK installs providers with `gizcli.Client.HandleDeviceControl(gizcli.DeviceControlHandlers{...})`, where a handler returning `gizcli.ErrDeviceRejected` / `gizcli.ErrDeviceResourceNotFound` maps to `INVALID_PARAMS` / `NOT_FOUND` (the OTA provider is `UpdateFirmware`); the Flutter SDK installs them through `GizClawPeerRpcHandlers.deviceControl` (`GizClawDeviceControlHandlers`), where a handler throws `GizClawDeviceControlException` to choose the RPC error code. Both answer `METHOD_NOT_FOUND` for an uninstalled handler.

## Music player

A device's single player provides seven `client.device.audioplayer.*` methods: `get` (113), `playlist.get` (114), `playlist.set` (115), `playlist.append` (116), `play` (117), `stop` (118), and `mode.set` (119). There is no `play_id`. The device's `playlist_revision` identifies a list version, not an append retry token.

| Method | Device behavior |
| --- | --- |
| `get` | Return the complete player status |
| `playlist.get` | Read the actual device playlist and revision |
| `playlist.set` | Validate, atomically replace, and stop; an empty list clears; failure preserves playback and the old list |
| `playlist.append` | Atomically append in order, allowing duplicates, without interrupting or starting playback |
| `play` | Require a zero-based `index`; start the selected track from its beginning, replacing current playback |
| `stop` | Stop idempotently while retaining the list and repeat mode |
| `mode.set` | `off` stops after the list, `one` repeats the track, `all` repeats the list; do not interrupt the current track |

A playlist contains at most 32 items. Each item has an HTTPS audio `url` without credentials or fragments (at most 1024 UTF-8 bytes), plus optional `title` and opaque `source_ref` (128 bytes each). The Server neither resolves the catalog nor downloads audio. The device downloads, decodes, plays, validates the complete request and reserves capacity before changing the list. Download or format failures appear in player status. List mutations increment `playlist_revision`; playback and mode changes do not. Persistence is device-owned; read `playlist.get` after reconnecting to discover the actual list.

A successful `play` only acknowledges acceptance. Devices report telemetry `audioplayer` observations (field 15) for `stopped`, `buffering`, `playing`, `ended`, and `error`, with the current index, actual playout `position_ms`, optional `duration_ms`, repeat mode, list length, and revision. Omit unknown duration. Millisecond integers must fit JavaScript's safe integer range. Only the error state carries `error_code` (128 bytes) and `error_message` (512 bytes); diagnostics must not contain URL credentials. Report transitions promptly and progress periodically while playing.

The Server stores `PeerStatus.audioplayer` in the existing KV snapshot, rejects older observations overwriting newer ones, and creates no player metric series in Prometheus. RPC status responses update the same snapshot; an omitted device wall clock uses server receipt time. Read `/device/status` for the snapshot; player `get` contacts the online device. Go providers use `DeviceControlHandlers.AudioPlayer`; JavaScript and Flutter use `deviceControl.audioplayer`; C uses the existing `rpc_provider` and bounded nanopb messages. Go, JavaScript, and C telemetry interfaces support the player observation.
