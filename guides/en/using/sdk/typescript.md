# TypeScript SDK <Badge type="warning" text="WIP" />

GizClaw ships two npm packages, split by role, both distributed as `v*` GitHub Release assets:

| Package | Directory | Role | Transport |
| --- | --- | --- | --- |
| `@gizclaw/gizclaw` | `sdk/js/gizclaw` | Device side: run a browser or Node process as a GizClaw device/Peer, plus Admin HTTP, RPC, signaling, and Telemetry | encrypted `/webrtc/v1/offer` signaling and WebRTC DataChannels |
| `@gizclaw/gizclaw-control` | `sdk/js/gizclaw-control` | Controller side: read and control the bound device with an [API key](../api-keys) | HTTPS `/gizclaw/v1/*` |

`@gizclaw/gizclaw` covers the same side as the C SDK in `sdk/c/gizclaw`. Device-side client initialization, runtime requirements, and RPC calls are not documented yet; this page covers `@gizclaw/gizclaw-control` and device handshake admission credentials.

## Install `@gizclaw/gizclaw-control`

From a selected [GitHub Release](https://github.com/GizClaw/gizclaw/releases), download
`npm-gizclaw-<version>.tgz`, `npm-gizclaw-control-<version>.tgz`,
`release-manifest.json`, and `SHA256SUMS`. Verify the package name, version, byte size,
SHA-256, and source commit against the manifest and the corresponding digests in
`SHA256SUMS`, then install both local tarballs in the consuming project:

```sh
# Set VERSION to the downloaded Release version, without the tag's leading v.
npm install "./npm-gizclaw-${VERSION}.tgz" "./npm-gizclaw-control-${VERSION}.tgz"
```

Both package versions equal the Release tag without `v`. The control package reuses
the generated Public HTTP client through `@gizclaw/gizclaw/peerhttp` and depends on
that exact gizclaw version. Passing both tarballs lets npm satisfy this dependency
locally. For the device SDK alone, install only `./npm-gizclaw-${VERSION}.tgz`.
The runtime needs `fetch`, `Request`, `Response`, and `URL`: Node `^22.13.0 || >=23.5.0` or a modern browser.

## Initialize and call

```ts
import { createGizClawControlClient } from "@gizclaw/gizclaw-control";

const control = createGizClawControlClient({
  baseUrl: "https://ap.gizclaw.com",
  apiKey, // gizclaw_sk_v1_...
});

const status = await control.device.getStatus();
const tools = await control.device.listTools();
console.log(status.volume, tools.tools);
```

Every request carries the API key, so `baseUrl` must be `https`. Only a local
test deployment sets `allowInsecureTransport: true` to reach a plaintext `http`
server, which sends the credential in the clear.

The client is organized by route group, with method names that mirror the `gizclaw_control` package in the [Flutter SDK](./flutter):

- `apiKeys`: `create`, `list`, `getSelf`, `revokeSelf`, `get`, `revoke`.
- `device`: `get`, `getRuntime`, `getStatus`, `getTelemetryLatest`, `queryTelemetry`, `aggregateTelemetry`, `setVolume`, `playSound`, `find`, `reboot`, `getWifi`, `scanWifi`, `connectWifi`, `listSavedWifi`, `forgetSavedWifi`, `getSettings`, `updateSettings`, `factoryReset`, `listRpcMethods`, `setRunWorkspace`, `listTools`, `invokeTool`.
- `contacts`: `list`, `create`, `get`, `put`, `delete`.
- `friends`: `getInviteToken`, `createInviteToken` (optional `{ ttl_seconds }`), `clearInviteToken`, `add`, `list`, `get`, `delete`.
- `friendGroups`: `list`, `create`, `join`, `get`, `put`, `delete` (dissolve), `leave`, `getInviteToken`, `createInviteToken`, `clearInviteToken`, `listMembers`, `addMember`, `putMember`, `deleteMember`.

Each members-list item carries optional `online` (Server-local connection state) and `last_seen_at` (RFC 3339 UTC; absent when unknown or the read failed); members returned by add, put and join omit both.

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

## Device handshake admission

`connectGiznetWebRTCFromEndpoint({ ..., credential })` and
`prepareEncryptedGiznetWebRTCOffer(identity, offerSDP, credential)` accept an
`AdmissionCredential` object with `{ version, type, value }`. The SDK uses the generated
protobuf-es codec and seals the result inside AEAD; Giznet does not interpret the fields.
Omission preserves bare SDP. Field and encoded-size limits are defined by
[Giznet](../../developing/giznet#signaling-admission-credentials).

The GizClaw package exports `registrationTokenCredential(registrationToken)`, returning
`{ version: 1, type: "gizclaw.com/registration_token", value: registrationToken }` for the connection options.
Callers must still invoke `server.register` after connecting to bind product resources.
Oversized or empty-encoding structures are rejected before discovery or offer creation,
closing the supplied PeerConnection. Operator settings are described in
[Security Policy](../../developing/gizclaw/server/security-policy).

The exported `REGISTRATION_TOKEN_CREDENTIAL_TYPE` constant defines the built-in type and is used by the helper. Values are limited to 512 UTF-8 bytes; construction returns an error or throws for larger input. Custom policies should use their own domain prefix; built-in types reserve `gizclaw.com/`.

## MHS v0 hardware states

Devices install `readMhsStates`/`writeMhsStates` on `deviceControl`; RPC values use `{bool_value:false}`, `{int_value:0}`, `{double_value:0}` or `{string_value:""}`. Controllers use `control.device.getMhsManifest()`, `readMhsStates({states:[{device_id,state}]})` and `writeMhsStates({states:[{device_id,state,value}]})`, with plain JSON values and generated Peer HTTP types.

This is GizClaw's MHS-inspired pre-standard v0, with no official compatibility claim. Manifests work offline; reads/writes allow at most 32 unique keys. Drivers validate the whole batch and enforce safety limits. See [Public API](/en/developing/api/http/public) and the [provider contract](/en/developing/api/proto/rpc/client-provided-to-server).

## tool/v0 procedures

Device providers install handlers for the predefined `ClientTool` values they implement. `client.tool.v0.list` reports that installed subset; typed control calls use the single `client.tool.v0.invoke` RPC. Hardware state uses the bound RuntimeProfile MHS v0 manifest and its read/write calls.
