# Flutter SDK <Badge type="warning" text="WIP" />

GizClaw ships two Dart packages, split by role:

| Package | Directory | Role | Transport | Dependencies |
| --- | --- | --- | --- | --- |
| `gizclaw` | `sdk/flutter/gizclaw` | Device side: run a Flutter app as a GizClaw device/Peer | encrypted `/webrtc/v1/offer` signaling and WebRTC DataChannels | Flutter, `flutter_webrtc`, `protobuf` |
| `gizclaw_control` | `sdk/flutter/gizclaw_control` | Controller side: read and control the bound device with an [API key](../api-keys) | HTTPS `/gizclaw/v1/*` | Pure Dart, `http` only |

`gizclaw` covers the same side as the C SDK in `sdk/c/gizclaw`. `gizclaw_control` targets phone controller apps such as LiteLink, where each device card stores one API key. Device-side client initialization, platform permissions, and RPC calls are not documented yet; this page covers `gizclaw_control`.

## Install `gizclaw_control`

Both packages are `publish_to: none` and are consumed as git dependencies on the repository path:

```yaml
dependencies:
  gizclaw_control:
    git:
      url: https://github.com/GizClaw/gizclaw.git
      ref: main
      path: sdk/flutter/gizclaw_control
```

`ref` may be a branch or a repository tag; pin a released app to a tag that contains the package. The package does not depend on Flutter and also works in Dart CLI and server code.

## Initialize and call

```dart
import 'package:gizclaw_control/gizclaw_control.dart';

final client = GizClawControlClient(
  baseUrl: Uri.parse('https://ap.gizclaw.com'),
  apiKey: apiKey, // gizclaw_sk_v1_...
);

final status = await client.getDeviceStatus();
final applied = await client.setDeviceVolume(level: 35, muted: false);
print('${status.volume} -> ${applied.status.volume}');

client.close();
```

Every request carries the API key, so `baseUrl` must be `https`. Only a local
test deployment sets `allowInsecureTransport: true` to reach a plaintext `http`
server, which sends the credential in the clear.

`GizClawControlClient` covers every `/gizclaw/v1/*` route:

- API keys: `createApiKey`, `listApiKeys`, `getSelfApiKey`, `revokeSelfApiKey`, `getApiKey`, `revokeApiKey`.
- Device reads: `getDevice`, `getDeviceRuntime`, `getDeviceStatus`, `getDeviceFirmware`, `getDeviceTelemetryLatest`, `queryDeviceTelemetry`, `aggregateDeviceTelemetry`.
- Device control: `setDeviceVolume`, `playDeviceSound`, `rebootDevice`, `updateDeviceFirmware`, `getDeviceWifi`, `scanDeviceWifi`, `connectDeviceWifi`, `listDeviceSavedWifi`, `forgetDeviceSavedWifi`.
- Contacts: `listContacts`, `createContact`, `getContact`, `putContact`, `deleteContact`.

Every method sends `Authorization: Bearer <apiKey>` and returns the immutable model for the contract type; `204` routes return `Future<void>`. Models ignore unknown JSON keys, and the open-ended schemas (`PeerStatus`, `DeviceInfo`) also expose `raw` with the complete decoded object. Path parameters (`ssid`, `contactName`) are URL-encoded by the SDK. The optional `httpClient` injects or shares an `http.Client`; `timeout` defaults to 30 seconds.

## Error handling

Every failure throws `GizClawControlException`, whose `kind` follows the error contract described under [API keys](../api-keys#device-reads-and-control):

| `kind` | Condition |
| --- | --- |
| `unauthorized` / `forbidden` / `notFound` | `401` / `403` / `404` |
| `deviceOffline` | `409 DEVICE_OFFLINE` |
| `deviceTimeout` | `504 DEVICE_TIMEOUT` |
| `deviceRejected` | `400 DEVICE_REJECTED` |
| `deviceUnsupported` | `501 DEVICE_UNSUPPORTED` |
| `deviceError` | `502 DEVICE_ERROR` |
| `conflict` / `invalidRequest` / `server` | any other `409` / `400` / `5xx` |
| `unexpectedStatus` | any other non-2xx |
| `malformedResponse` | 2xx whose body does not match the contract |
| `network` | DNS, socket, TLS, or timeout failure with no HTTP response |

`DEVICE_*` kinds match the `error.code` in the response body; the rest match the HTTP status. The exception also carries `statusCode`, `code`, `message`, `details`, and `requestId` from the `X-Request-ID` response header.

```dart
try {
  await client.playDeviceSound(sound: 'chime');
} on GizClawControlException catch (e) {
  switch (e.kind) {
    case GizClawControlErrorKind.deviceOffline:
      // Tell the user the device is offline and retry later.
    case GizClawControlErrorKind.unauthorized:
      // The key was revoked; ask the user to re-pair.
    default:
      // Show e.message.
  }
}
```
