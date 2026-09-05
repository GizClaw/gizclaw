# Public API

Public API is an HTTP contract that Server exposes to Public/Peer caller before and after WebRTC connection is established. It is the entry boundary and does not represent the full capabilities of the Peer domain service.

Source:`api/http/peer.json`
Go generated output: `pkgs/gizclaw/api/peerhttp`

See the [API Reference](/api/) for exact endpoints, parameters, requests, and responses. This page only explains the Public/Peer surface design boundary.

`/webrtc/v1/offer` Occurs before the Peer connection is established, HTTP signaling must be preserved. The Peer capability after establishing a connection can use reliable HTTP-over-service-stream or Peer RPC; when choosing a transport, avoid maintaining two sets of contracts for the same capability.

The Offer is authenticated by the signed signaling contract itself and does not depend on an API key. Public API can reuse real shared types such as `ErrorResponse`, `DeviceInfo` and `Runtime`, but does not reference Admin Resources.

See [Peer HTTP · API keys](../../gizclaw/peer/service/api-keys) for the authentication and management contract. First-time provisioning stays on the device-local BLE channel; once the device is online, an API key can scan or change Wi-Fi through `/gizclaw/v1/device*`.

## Device and contact surface

`/gizclaw/v1/device*` and `/gizclaw/v1/contacts*` accept `Authorization: Bearer <api-key>` or device debug access as described below. The Server takes the immutable owner Peer from the key record; the `public_key` debug query does not override the API key owner, and manager keys and ordinary keys have the same owner-scoped capability on these routes. An invalid or revoked key answers `401 INVALID_API_KEY`, an owner that is not an active Client with a RuntimeProfile binding answers `403 API_KEY_OWNER_UNAVAILABLE`, an owner pending deletion answers `409 PEER_PENDING_DELETION`, validation and pagination failures answer `400 INVALID_REQUEST`, and store or service failures collapse into a redacted `500 INTERNAL_ERROR`.

Read routes project the authoritative services and never send an RPC to the device:

- `GET /device` returns `DeviceInfo` (name, emoji, `HardwareInfo`, `DeviceIdentifiers`), the same source as `server.info.get`.
- `GET /device/runtime` returns `Runtime` (online, last seen, address, RX/TX) without refreshing online state.
- `GET /device/status` returns the latest authoritative `PeerStatus` snapshot. There is no `fresh` parameter; `client.device.status.get` exists only for control-response write-back.
- `GET /device/telemetry/latest`, `/device/telemetry`, and `/device/telemetry/aggregate` keep the Admin telemetry field enum, observation times, query limits, ordering, and aggregate semantics and pin the Peer to the owner.
- `/contacts` list/create/get/put/delete use the same owner-scoped data as `services/social/contact`; `{contactName}` is the owner-scoped immutable `name`, cross-owner and missing names both answer `404 CONTACT_NOT_FOUND`, and name or phone conflicts answer `409 CONTACT_ALREADY_EXISTS`.

## Device control flow

Control routes are forwarded as Server-to-device RPCs (see [Client Provided to Server](../proto/rpc/client-provided-to-server)):

```text
PUT /gizclaw/v1/device/volume { level: 0..100, muted }
  → resolve the API key owner
  → find the owner's active connection; none → 409 DEVICE_OFFLINE
  → client.device.volume.set, 5s timeout → 504 DEVICE_TIMEOUT
  → device reports PeerStatus → stored as the owner's PeerStatus (reported_at from the device)
  → 200 { status: PeerStatus }
```

| Route | RPC | Success |
| --- | --- | --- |
| `PUT /device/volume` | `client.device.volume.set` | `200 { status }` |
| `POST /device/actions/play-sound` `{ sound, duration_ms? }` | `client.device.sound.play` | `204` |
| `POST /device/actions/reboot` `{ delay_ms? }` | `client.device.reboot` | `204` |
| `GET /device/wifi` | `client.wifi.status.get` | `200 DeviceWifiStatus` |
| `GET /device/wifi/saved` | `client.wifi.saved.list` | `200 DeviceWifiSavedList` |
| `DELETE /device/wifi/saved/{ssid}` | `client.wifi.saved.forget` | `204`; unknown ssid → `404 WIFI_NETWORK_NOT_FOUND` |
| `POST /device/wifi/scan` `{ timeout_ms? }` | `client.wifi.scan` | `200 { networks }` |
| `PUT /device/wifi` `{ ssid, passphrase? }` | `client.wifi.connect` | `202` |

`sound` is a device-defined string: the Server only checks that it is non-empty and at most 32 UTF‑8 bytes, and the device provider validates the value; `ssid` has the same 32-byte bound. Wi-Fi scan defaults `timeout_ms` to 8000 and clamps it to 1000–15000 instead of using the normal 5-second control timeout. Omit `passphrase` for an open network; a PSK is 8–63 bytes. A `202` connect response only means the device accepted the credentials: it answers RPC before switching, then necessarily goes offline. Control routes answer `409 DEVICE_OFFLINE` during the outage. After reconnect, clients poll `GET /device/wifi` and compare `ssid` with the target to distinguish success from fallback. The passphrase is only forwarded and is never persisted, logged, or echoed. Scan results come from the device, so the Server revalidates them before answering: at most 32 entries, a non-empty `ssid` of at most 32 bytes, a `bssid` of at most 17, and a `security` of at most 5. An answer outside those bounds is rejected whole as `502 DEVICE_ERROR` without echoing the offending value.

A device `INVALID_PARAMS` maps to `400 DEVICE_REJECTED`, `METHOD_NOT_FOUND` (no provider implemented) maps to `501 DEVICE_UNSUPPORTED`, and every other RPC error maps to a redacted `502 DEVICE_ERROR`; bodies carry only a stable `code` and a redacted `message`. Concurrent control commands for one owner are forwarded serially in arrival order and are never merged or replayed. After a device acknowledges `reboot` or `wifi.connect`, later control commands on that same connection answer `409 DEVICE_OFFLINE` until the device reconnects. Control commands never change PeerRun, Workspace, or Agent state.

Before connection, `/server-info` reports the authoritative Server's `public_key`, software `version`, `build_commit`, and transport capabilities. Server identity remains the cryptographic `public_key`. Through an Edge, the build fields remain those of the authoritative Server, while the `transport` object alone selects the Edge route.

## Device debug access and anonymous lookup

An authenticated device sets its own `debug_mode` through `server.info.put`:
`off` (default), `readonly`, or `fullcontrol`. Omission preserves the stored mode;
invalid values are rejected. The mode lives in the device record in PeerStore and
is not overwritten by reverse identifier refresh.

Without Authorization, device and contact HTTP routes accept `?public_key=<pubkey>`.
`readonly` permits GET; `fullcontrol` permits reads, writes, and device controls on
these routes. The Server reads the mode on every request; disabling it rejects new
anonymous requests, without canceling requests already in progress. Active Client
status and RuntimeProfile binding remain required. API key management, Admin, and
OpenAI routes do not accept debug authorization. An invalid Authorization header
never falls back to debug access; valid API keys retain their own immutable owner.
Debug responses carry `Cache-Control: no-store`. Edge uses the existing Peer
assignment to forward to the configured owning Server, which enforces access.

Anonymous GET `/gizclaw/v1/peers/@findBySn/{sn}` and
`/gizclaw/v1/peers/@findByImei/{tac}/{serial}` return
`{"public_keys": [...]}` for every match, including devices with debug disabled.
No match returns an empty array; no device metadata is included. Both identifiers
are non-unique device declarations. IMEI indexes use
`by-imei:<tac>:<serial>:<pubkey>` and validate prefix matches against current records.
Admin lookup is `/peers/@findPubKeysByImei/{tac}/{serial}`; it and CLI
`admin peers resolve-imei` return all matching public keys.
