# API keys

GizClaw uses long-lived, device-bound API keys for the public GizClaw and OpenAI-compatible HTTP APIs. A registered device manages its keys over the authenticated Peer RPC connection with `server.api_key.create`, `server.api_key.list`, and `server.api_key.revoke`. The Peer connection is the root authority. API keys are recoverable management resources: create, list, get, and self responses include the complete `gizclaw_sk_v1_...` credential.

Send the secret as `Authorization: Bearer <api-key>` to `/gizclaw/v1/*` and `/openai/v1/*`. No public-key header or login exchange is required.

An ordinary key can use public APIs, inspect itself with `GET /gizclaw/v1/api-keys/self`, and revoke itself with `DELETE /gizclaw/v1/api-keys/self`. A key created with `manage_api_keys: true` can also create, list, inspect, and revoke other keys belonging to the same device through `/gizclaw/v1/api-keys`. `manage_api_keys` only governs key management; every key can read and control its bound device with identical capability.

## Device reads and control

The key's bound device is the fixed target of every `/gizclaw/v1/device*` and `/gizclaw/v1/contacts*` request (see [API](./api#device-http-api) for the route list). Read the device status and set the volume:

```sh
curl -sS "$GIZCLAW_URL/gizclaw/v1/device/status" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY"

curl -sS -X PUT "$GIZCLAW_URL/gizclaw/v1/device/volume" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"level":35,"muted":false}'

curl -sS -X POST "$GIZCLAW_URL/gizclaw/v1/device/wifi/scan" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"timeout_ms":8000}'

curl -sS -X PUT "$GIZCLAW_URL/gizclaw/v1/device/wifi" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"ssid":"Office","passphrase":"correct-horse"}'
```

A successful `PUT /device/volume` answers `200 { "status": PeerStatus }` whose `volume` and `muted` match the next `GET /device/status`; `play-sound`, `reboot`, and `DELETE /wifi/saved/{ssid}` answer `204`. A Wi-Fi scan synchronously returns nearby networks; `timeout_ms` defaults to 8000 and is clamped to 1000–15000. Connecting answers `202`, which only means the device accepted the credentials and started switching. The device then goes offline and control routes answer `409 DEVICE_OFFLINE`. After reconnect, poll `GET /device/wifi`: the target `ssid` means success, while the old one means the device failed and fell back. The Server never stores, logs, or echoes the passphrase.

When the device is offline, control routes answer `409 DEVICE_OFFLINE`; normal controls that do not answer within 5 seconds and scans that exceed their requested bound answer `504 DEVICE_TIMEOUT`; neither changes the stored status. After a `reboot` is acknowledged, control requests answer `409` until the device reconnects. A device that rejects the parameters answers `400 DEVICE_REJECTED`, and firmware without the capability answers `501 DEVICE_UNSUPPORTED`. Once the key is revoked or the device Peer is deleted, every device and contact request fails immediately.

## SDKs

The controller-side SDKs wrap the same routes and error contract: `gizclaw_control` for Dart ([Flutter SDK](./sdk/flutter)) and `@gizclaw/gizclaw-control` on npm ([TypeScript SDK](./sdk/typescript)). Read the device status and set the volume:

```dart
import 'package:gizclaw_control/gizclaw_control.dart';

final client = GizClawControlClient(
  baseUrl: Uri.parse('https://ap.gizclaw.com'),
  apiKey: apiKey,
);
final status = await client.getDeviceStatus();
final applied = await client.setDeviceVolume(level: 35, muted: false);
final nearby = await client.scanDeviceWifi();
await client.connectDeviceWifi(
  const DeviceWifiConnectRequest(ssid: 'Office', passphrase: 'correct-horse'),
);
```

```ts
import { createGizClawControlClient } from "@gizclaw/gizclaw-control";

const control = createGizClawControlClient({
  baseUrl: "https://ap.gizclaw.com",
  apiKey,
});
const status = await control.device.getStatus();
const applied = await control.device.setVolume({ level: 35, muted: false });
const nearby = await control.device.scanWifi();
await control.device.connectWifi({ ssid: "Office", passphrase: "correct-horse" });
```

Failures throw `GizClawControlException` and `GizClawControlError` respectively, whose `kind` classifies the status and `DEVICE_*` codes above, such as `deviceOffline`, `deviceTimeout`, and `unauthorized`.

API keys do not expire automatically. Revoke keys that are lost or no longer needed. Deleting a Peer revokes all keys owned by that Peer.
