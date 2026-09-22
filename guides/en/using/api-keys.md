# API keys

GizClaw uses long-lived, device-bound API keys for the public GizClaw and OpenAI-compatible HTTP APIs. A registered device manages its keys over the authenticated Peer RPC connection with `server.api_key.create`, `server.api_key.list`, and `server.api_key.revoke`. The Peer connection is the root authority. API keys are recoverable management resources: create, list, get, and self responses include the complete `gizclaw_sk_v1_...` credential.

Send the secret as `Authorization: Bearer <api-key>` to `/gizclaw/v1/*` and `/openai/v1/*`. No public-key header or login exchange is required.

An ordinary key can use public APIs, inspect itself with `GET /gizclaw/v1/api-keys/self`, and revoke itself with `DELETE /gizclaw/v1/api-keys/self`. A key created with `manage_api_keys: true` can also create, list, inspect, and revoke other keys belonging to the same device through `/gizclaw/v1/api-keys`. `manage_api_keys` only governs key management; every key can read and control its bound device with identical capability.

## Device reads and control

The key's bound device is the fixed target of every `/gizclaw/v1/device*`, `/gizclaw/v1/contacts*`, `/gizclaw/v1/friends*`, and `/gizclaw/v1/friend-groups*` request (see [API](./api#device-http-api) for the route list). Read the device status and write a hardware state::

```sh
curl -sS "$GIZCLAW_URL/gizclaw/v1/device/status" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY"

curl -sS -X PATCH "$GIZCLAW_URL/gizclaw/v1/device/mhs/v0/states" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"states":[{"device_id":"speaker.main","state":"volume","value":35}]}'

curl -sS -X POST "$GIZCLAW_URL/gizclaw/v1/device/tool/v0/invoke" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"tool":"wifi.scan","args":{"timeout_ms":8000}}'

curl -sS -X POST "$GIZCLAW_URL/gizclaw/v1/device/tool/v0/invoke" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"tool":"wifi.connect","args":{"ssid":"Office","passphrase":"correct-horse"}}'

```

A valid MHS write returns the applied states. `tool/v0` invokes return a typed `result`; `wifi.scan` returns nearby networks and `wifi.connect` acknowledges the request before switching networks. Check the manifest and installed tool list before showing controls. The Server never stores, logs or echoes the Wi-Fi passphrase.

When the device is offline, control routes answer `409 DEVICE_OFFLINE`; normal controls that do not answer within 5 seconds and scans that exceed their requested bound answer `504 DEVICE_TIMEOUT`; neither changes the stored status. After a `reboot` is acknowledged, control requests answer `409` until the device reconnects. A device that rejects the parameters answers `400 DEVICE_REJECTED`, and firmware without the capability answers `501 DEVICE_UNSUPPORTED`. Once the key is revoked or the device Peer is deleted, every device and contact request fails immediately.

## SDKs

The controller-side SDKs wrap the same routes and error contract: `gizclaw_control` for Dart ([Flutter SDK](./sdk/flutter)) and `@gizclaw/gizclaw-control` on npm ([TypeScript SDK](./sdk/typescript)). Read the device status and write a hardware state::

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
