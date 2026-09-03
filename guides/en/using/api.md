# API

GizClaw exposes two primary interfaces for administrators and peers. The Admin API manages the Server as a whole, while Peer RPC lets a connected peer call product capabilities. Both reuse an authenticated Giznet peer connection; the Admin API is not an unauthenticated public HTTP endpoint. A phone app or script holding a device API key can additionally read and control its bound device over Public HTTP; see [Device HTTP API](#device-http-api) below.

## Choosing an interface

| Interface | Intended caller | Contract | Giznet service | Typical uses |
| --- | --- | --- | --- | --- |
| Admin API | Operators, CLI, and management UIs | OpenAPI 3.0 / HTTP | `0x10` (Admin HTTP) | Peer administration, declarative resources, provider configuration, firmware, telemetry, and Server logs |
| Peer RPC | Devices, apps, and SDKs | Protobuf RPC | `0x00` (Peer RPC) | Runtime state, workspaces, workflows, firmware, social data, gameplay, and device capabilities |

Use the Admin API for Server resources that span peers. Use Peer RPC to read or modify product data as the current peer. Edge-node route control uses the separate Edge RPC service `0x31`; it is not part of the ordinary Peer RPC client.

Before calling either interface, persist the caller's own keypair and obtain the Server access point and public key. The access point is an `http://` or `https://` base URL such as `https://ap.gizclaw.com`; a bare `host:port` still resolves to `http`. The examples below assume that the SDK has already connected: a dialed `*gizcli.Client` in Go, or an `RTCPeerConnection` established by `connectGiznetWebRTCFromEndpoint` in TypeScript. WebRTC media does not reuse the access point authority, because a TLS access point can terminate on a port that carries no ICE: the SDK takes the ICE UDP address from the `endpoint` field of `/server-info`. Never log or commit private keys, login assertions, or session credentials.

The TypeScript SDK is published to GitHub Packages as `@gizclaw/gizclaw`. Add
the following entries to the consuming project's `.npmrc`, provide a GitHub
token with `read:packages` as `GITHUB_PACKAGES_TOKEN`, and install an explicit
version with `npm install @gizclaw/gizclaw@0.7.2`:

```ini
@gizclaw:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${GITHUB_PACKAGES_TOKEN}
```

A commit on `main` publishes only when it changes publishable SDK content and
increases the stable SemVer in `sdk/js/gizclaw/package.json`; an existing
version is never replaced.

## Admin API

The Admin API preserves HTTP methods, paths, headers, JSON/YAML bodies, status codes, and SSE semantics, but requests travel over the Admin HTTP service. SDKs use the virtual base URL `http://gizclaw` to construct requests; it is not a DNS name or a directly exposed Server address.

The Server permits only these identities to open the Admin HTTP service:

- the bootstrap admin key selected by `admin-public-key`;
- a registered, active peer with the `admin` role.

Ordinary devices and apps must not hold the admin key. Prefer the `gizclaw admin` CLI for routine operator work. Use the generated Admin client when integrating a management UI or automation.

### Capability groups

- Declarative resources: `POST /@apply` and `/resources/{kind}/{name}`.
- Peers: query, approve, block, refresh, device information, and runtime.
- AI and runtime: credentials, provider tenants, models, voices, workflows, workspaces, runtime profiles, and registration tokens.
- Firmware and gameplay: firmware channel package configuration, game definitions, pet definitions, badge definitions, and peer gameplay data.
- Operations: peer telemetry queries, the Server log SSE stream, and active pending-deletion inspection/retry.

See [`api/http/admin.json`](https://github.com/GizClaw/gizclaw/blob/main/api/http/admin.json) for the complete paths, parameters, and responses.

### TypeScript

```ts
import {
  createAdminAPIClient,
  listPeers,
} from "@gizclaw/gizclaw/admin";

const admin = createAdminAPIClient(pc);
const peers = await listPeers({
  client: admin,
  responseStyle: "data",
  throwOnError: true,
});
```

`createAdminAPIClient` binds the generated OpenAPI client to the existing peer connection. Generated operation functions provide typed paths, queries, bodies, and responses. For non-success responses, use `throwOnError: true` or explicitly inspect `data`, `error`, and the underlying `Response`.

### Pending-deletion operations

Use `listPendingDeletions` to inspect active tasks, then pass both the returned `source` and `deletion_id` to `getPendingDeletion` or `retryPendingDeletion`. Retry is accepted only for `failed` work. `queued`, `running`, and `retry_wait` return `409`. After physical deletion succeeds, the task is removed immediately and get/retry returns `404`; a successful deletion has no receipt to query. Treat `resource_id`, bounded error code/message, status, and timestamps as operator data. Owner identity, descriptors, payloads, lease tokens, fingerprints, and raw backend errors are intentionally absent.

### Go

```go
admin, err := client.ServerAdminClient()
if err != nil {
	return err
}

response, err := admin.ListPeersWithResponse(
	ctx,
	&adminhttp.ListPeersParams{},
)
if err != nil {
	return err
}
if response.JSON200 == nil {
	return fmt.Errorf("list peers: status %d", response.StatusCode())
}
peers := response.JSON200.Items
```

`ServerAdminClient` returns the generated `adminhttp.ClientWithResponses`. Callers must handle both transport errors and unexpected HTTP status codes. Read a success body only when the corresponding typed response field is non-`nil`.

## Peer RPC

Peer RPC sends Protobuf requests, responses, errors, and stream frames over a peer connection. Every call has a request ID, a stable method name, and a method-specific payload. Use the generated typed method map or the Go SDK methods instead of hard-coding method numbers or encoding payloads manually.

Inside method payloads, addressable object identity is always `name`, and references use `<kind>_name`. If an object has no separate Peer alias, the Server returns its canonical or internal record ID verbatim in `name`; callers still use the `name` field. `display_name` is presentation text and `actor_name` is attribution. An `id` field in the RPC envelope correlates frames and is not a business-object identity.

A method-name prefix identifies the capability provider:

- `all.*`: common capabilities provided by both Client and Server, such as `all.ping`.
- `server.*`: product capabilities provided by the Server and called by a Client.
- `client.*`: device capabilities provided by a Client and called by the Server.
- `runtime.*`: calls owned by the runtime contract.

See the [RPC API Reference](/references/rpc) for the purpose of every available method. The wire contract, method IDs, and error codes remain authoritative in [`api/proto/rpc/rpc.proto`](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/rpc.proto); request and response messages are defined under `api/proto/rpc/payload/`.

### TypeScript

```ts
import {
  createPeerRPCClient,
  RPC_METHODS,
} from "@gizclaw/gizclaw/rpc";

const rpc = createPeerRPCClient(pc);
const status = await rpc.call(
  RPC_METHODS["server.run.status"],
  {},
  { timeoutMs: 10_000 },
);
```

`createPeerRPCClient` accepts only Peer RPC methods; generated mappings associated with `RPC_METHODS` constrain request and response types. Methods that return metadata together with firmware, icons, audio, or other bytes use `callBinary`. A call can take an `AbortSignal` or a per-call `timeoutMs`; the default request timeout is 30 seconds.

### Go

```go
status, err := client.GetServerStatus(ctx, "status-request-1")
if err != nil {
	return err
}
```

The Go SDK exposes common RPCs as typed methods on `gizcli.Client`. The supplied request ID must be unique among concurrent calls on the current connection. Use the `context` for cancellation and deadlines.

## Device HTTP API

An API key bound to one device (see [API keys](./api-keys)) can reach `/gizclaw/v1/device*` and `/gizclaw/v1/contacts*` without a Peer connection, through Direct Server HTTP (`serve-to-clients=true`) or Edge HTTPS. Send `Authorization: Bearer <api-key>` on every request; resources and commands always target the key's bound device and cannot name another Peer.

| Route | Purpose |
| --- | --- |
| `GET /gizclaw/v1/device` | Device name, emoji, hardware info, and identifiers |
| `GET /gizclaw/v1/device/runtime` | Online state, last seen time, and traffic |
| `GET /gizclaw/v1/device/status` | Latest reported battery, charging, volume, mute, and GNSS |
| `GET /gizclaw/v1/device/telemetry/latest`, `/telemetry`, `/telemetry/aggregate` | Sampled telemetry queries with Admin telemetry semantics |
| `PUT /gizclaw/v1/device/volume` | Set volume and mute; returns the status the device reports |
| `POST /gizclaw/v1/device/actions/play-sound` | Play a device-defined sound |
| `POST /gizclaw/v1/device/actions/reboot` | Reboot the device |
| `GET /gizclaw/v1/device/wifi`, `/wifi/saved`, `DELETE /wifi/saved/{ssid}` | Query Wi‑Fi status, list and forget saved networks |
| `/gizclaw/v1/contacts`, `/contacts/{contactName}` | List/create/get/put/delete the device's contacts |

Read routes project data the Server already holds and never wake the device. Control routes execute live over a Server-to-device RPC: an offline device answers `409 DEVICE_OFFLINE`, no answer within 5 seconds gives `504 DEVICE_TIMEOUT`, and a device without the provider gives `501 DEVICE_UNSUPPORTED`. Poll `GET /device/status` for state changes. Wi‑Fi provisioning stays on the device-local BLE channel.

```sh
curl -sS -X PUT "$GIZCLAW_URL/gizclaw/v1/device/volume" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"level":35,"muted":false}'
```

TypeScript uses the generated `setDeviceVolume`, `getDeviceStatus`, `listContacts`, and related operations from `@gizclaw/gizclaw/peerhttp`; Go uses `peerhttp.ClientWithResponses` (the same type `gizcli.Client.PeerHTTPClient()` returns). The exact paths, parameters, and responses are defined by [`api/http/peer.json`](https://github.com/GizClaw/gizclaw/blob/main/api/http/peer.json).

## Errors and connection lifecycle

- Admin API HTTP error bodies use stable error codes. Do not branch on message text.
- RPC failures use `RpcErrorCode`. Standard codes include parse error, invalid request, method not found, invalid params, internal error, and `400`, `403`, `404`, and `409`.
- A transport error, timeout, or closed connection does not prove that a write had no effect. Read the final Server state before retrying a mutation.
- Close the `RTCPeerConnection` when a TypeScript session ends and the `gizcli.Client` when a Go session ends. Reconnect after a disconnect instead of reusing a stale client.

For design and generation details, see the [Admin API development guide](../developing/api/http/admin) and [Peer RPC development guide](../developing/api/proto/rpc/overview).
