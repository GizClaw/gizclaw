# TypeScript SDK <Badge type="warning" text="WIP" />

GizClaw ships two npm packages, split by role, both published to GitHub Packages:

| Package | Directory | Role | Transport |
| --- | --- | --- | --- |
| `@gizclaw/gizclaw` | `sdk/js/gizclaw` | Device side: run a browser or Node process as a GizClaw device/Peer, plus Admin HTTP, RPC, signaling, and Telemetry | encrypted `/webrtc/v1/offer` signaling and WebRTC DataChannels |
| `@gizclaw/gizclaw-control` | `sdk/js/gizclaw-control` | Controller side: read and control the bound device with an [API key](../api-keys) | HTTPS `/gizclaw/v1/*` |

`@gizclaw/gizclaw` covers the same side as the C SDK in `sdk/c/gizclaw`. Device-side client initialization, runtime requirements, and RPC calls are not documented yet; this page covers `@gizclaw/gizclaw-control`.

## Install `@gizclaw/gizclaw-control`

Point the `@gizclaw` scope at GitHub Packages in the project `.npmrc`, then install:

```ini
@gizclaw:registry=https://npm.pkg.github.com
```

```sh
npm install @gizclaw/gizclaw-control
```

The package depends on the `peerhttp` entry of `@gizclaw/gizclaw` for the generated Public HTTP client, which installs alongside it. The runtime needs `fetch`, `Request`, `Response`, and `URL`: Node `^22.13.0 || >=23.5.0` or a modern browser.

## Initialize and call

```ts
import { createGizClawControlClient } from "@gizclaw/gizclaw-control";

const control = createGizClawControlClient({
  baseUrl: "https://ap.gizclaw.com",
  apiKey, // gizclaw_sk_v1_...
});

const status = await control.device.getStatus();
const applied = await control.device.setVolume({ level: 35, muted: false });
console.log(status.volume, "->", applied.status.volume);
```

Every request carries the API key, so `baseUrl` must be `https`. Only a local
test deployment sets `allowInsecureTransport: true` to reach a plaintext `http`
server, which sends the credential in the clear.

The client is organized by route group, with method names that mirror the `gizclaw_control` package in the [Flutter SDK](./flutter):

- `apiKeys`: `create`, `list`, `getSelf`, `revokeSelf`, `get`, `revoke`.
- `device`: `get`, `getRuntime`, `getStatus`, `getTelemetryLatest`, `queryTelemetry`, `aggregateTelemetry`, `setVolume`, `playSound`, `reboot`, `getWifi`, `scanWifi`, `connectWifi`, `listSavedWifi`, `forgetSavedWifi`.
- `contacts`: `list`, `create`, `get`, `put`, `delete`.

Request and response types come straight from the generated types in `@gizclaw/gizclaw/peerhttp` (`PeerStatus`, `DeviceControlStatus`, `Contact`, and so on), so field names match the wire format. `204` routes resolve to `void`. `control.client` exposes the generated client already configured with the bearer token and `baseUrl`, ready to pass to other `@gizclaw/gizclaw/peerhttp` functions. The optional `fetch` option injects a custom or test fetch.

## Error handling

Every failure rejects with `GizClawControlError`, whose `kind` follows the error contract described under [API keys](../api-keys#device-reads-and-control):

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
| `network` | fetch threw; no HTTP response |

`DEVICE_*` kinds match the `error.code` in the response body; the rest match the HTTP status, and `classifyGizClawControlError(status, code)` is exported on its own. The error also carries `status`, `code`, `details`, `requestId` (the `X-Request-ID` response header), and `cause`.

```ts
import { GizClawControlError } from "@gizclaw/gizclaw-control";

try {
  await control.device.playSound({ sound: "chime" });
} catch (error) {
  if (error instanceof GizClawControlError && error.kind === "deviceOffline") {
    // Tell the user the device is offline and retry later.
  } else {
    throw error;
  }
}
```
