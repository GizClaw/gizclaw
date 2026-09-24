# TypeScript SDK <Badge type="warning" text="WIP" />

> This page describes the directories, contract boundaries, generation flow, and release contract of the two npm packages. The public surface and runtime behavior of `@gizclaw/gizclaw` are still to be expanded one by one.

`sdk/js/` is an npm workspace with two publishable packages and shared generation scripts:

- `sdk/js/gizclaw` is the device-side `@gizclaw/gizclaw`, covering Admin HTTP, Public HTTP, RPC, signaling, and Telemetry, and owning every generated client. It covers the same side as `sdk/c/gizclaw`.
- `sdk/js/gizclaw-control` is the controller-side `@gizclaw/gizclaw-control`, which binds one API key to the generated Public HTTP client and exposes the `/gizclaw/v1/*` route groups with error classification. It depends on `@gizclaw/gizclaw/peerhttp` instead of generating a second copy of the contract.
- `sdk/js/scripts` holds the tools that generate SDK surfaces from OpenAPI, Protobuf, and the method registry, normalize generated output, and check releases.

```text
sdk/js/
├── gizclaw/                 # @gizclaw/gizclaw: device-side SDK and generated clients
│   ├── peerhttp.ts          # Public HTTP generated surface; also exports createPeerHTTPClient
│   └── generated/           # Updated only by gen:sdk / gen:telemetry
├── gizclaw-control/         # @gizclaw/gizclaw-control: controller-side SDK
│   ├── index.ts             # createGizClawControlClient, GizClawControlError
│   └── index.test.ts        # Route and error-mapping tests with an injected fetch stub
└── scripts/                 # Contract generation, output normalization, release checks
```

The source of truth for generated content is [API Design](../api/overview); generated output is never maintained as a handwritten implementation.

## `@gizclaw/gizclaw`

Browser and Desktop clients connect through encrypted `/webrtc/v1/offer`
signaling and carry protobuf RPC envelopes, body frames, and EOS on the ordered
`giznet/v1/service/0` DataChannel. Before creating the offer,
`connectGiznetWebRTC` creates the packet DataChannel and an Opus-capable audio
transceiver; callers inject runtime-specific identity, crypto, and fetch
primitives.

`createWebRTCFetch` is the generated-client fetch adapter boundary. The current
WebRTC bridge maps HTTP requests to GizClaw RPC methods; it is not an arbitrary
HTTP proxy.

`serveGiznetWebRTCRPC(pc, handlers)` answers the Server's device RPCs. `GizClawPeerRPCHandlers` installs `mhs/v0` state handlers and predefined `ClientTool` procedures. `client.rpc.methods.list` advertises supported protocol families and `client.tool.v0.list` reports only installed procedures. An uninstalled tool answers `METHOD_NOT_FOUND`, which the Server maps to `501 DEVICE_UNSUPPORTED`. Handlers may throw `GizClawDeviceControlError` for a specific RPC status. The `peerRPCHandlers` connect option installs handlers before signaling.

## `@gizclaw/gizclaw-control`

`createGizClawControlClient` builds a separate generated client with `createPeerHTTPClient` (`baseUrl`, `auth`, optional `fetch`), one instance per API key, and never touches the `peerHTTPClient` singleton. Route methods call the `sdk.gen.ts` functions with `throwOnError: false` and convert `{ error, response }` into `GizClawControlError`: a missing `response` is `network`; otherwise the body's `error.code` is matched against `DEVICE_*` first and the status is classified second. The code constants are owned by `pkgs/gizclaw/peer_service_serve_peer_http_device_control.go`. Path parameters are `encodeURIComponent`-encoded by the generated client.

## Generation and validation

```sh
npm ci
npm --prefix sdk/js run gen:sdk
npm --prefix sdk/js test
npm test --workspace @gizclaw/gizclaw-control
npm run quality:typescript
npm run quality:lint
npm run quality:format
```

The `pretest` and `prebuild` scripts of `gizclaw-control` build `@gizclaw/gizclaw` first, because the package resolves `dist/peerhttp.*` through package exports.

## Release contract

Both packages ship only through the `js-sdk` job in `.github/workflows/release.yml`
when a canonical `vMAJOR.MINOR.PATCH` tag is released.
`.github/workflows/js-sdk-release.yml` only verifies development manifests and runs
SDK and tarball contract tests on pull requests and pushes to `main`.

`DEVELOPMENT_VERSION` in `sdk/js/scripts/check-package-release.mjs` is the single
source of truth for the development placeholder, `0.0.0`. Default mode checks the
selected package name, placeholder version, and matching workspace version in
`package-lock.json`, and rejects publication configuration targeting the retired
npm hosting service. Source dependencies from control to gizclaw and console to
control use `"*"` to resolve local workspaces. SDK changes require no manual package
version bump.

`tools/js-sdk/package_npm_tarball.sh` checks HEAD, source commit, source epoch,
and clean owned paths, builds `dist`, and lets `npm pack` select the files. It
injects the Release version into a temporary manifest, rewrites control's
`dependencies["@gizclaw/gizclaw"]` to that exact version, and removes `publishConfig`.
`check-package-release.mjs --package sdk/js/<name> --release-version <version> --manifest <path>` checks the injected identity and exact internal dependency
without requiring a workspace lockfile in the tarball. Packaging and archive
verification both reuse this mode.

The final tarball retains npm's entry set under `package/`, sorted by path in
USTAR format, with uid/gid 0, uname/gname root, file mode 0644, directory mode 0755,
and tar/gzip mtimes equal to the source epoch. `verify_npm_tarball.sh` checks the
structure, normalized metadata, and JS/type declarations for every public entry.
`consume_tarballs.sh` installs both local tgz files in a temporary project and
imports every entry. `archive_test.sh` builds twice, compares bytes with `cmp`,
and tests rejection of invalid archives.

`prepare-published-sdk.mjs <package>` runs after `tsc`, copies generated Protobuf
JavaScript when present, and rewrites `.ts` imports in `.d.ts` files to `.js`.
See [Repository Releases](../tooling#repository-releases) for asset names, manifest
schema, and downstream distribution ownership, and
[TypeScript SDK](/en/using/sdk/typescript) for installation.

## Monitor clients

`@gizclaw/gizclaw-control` exports `createGizClawPeerMonitorClient`,
`createGizClawNodeMonitorClient` and `createGizClawDiscoveryClient`.
Peer monitoring uses `Authorization: Bearer gizclaw_pk_<public key>` and the
owning Server enforces runtime readonly/fullcontrol/off permissions. Node
monitoring uses its independent Monitor Token. Public SN/IMEI discovery sends
no credential and returns all matches.

The peer client exposes device snapshots, Telemetry, `listWorkspaces`,
`listWorkspaceHistory`, `searchLogs` and `downloadHistoryAudio`. The same read
methods are available on `createGizClawControlClient(...).device` for API-key
users. `listWorkspaces` accepts optional `collection` and `workflow_name`
filters, and `deleteWorkspace` resolves once the Server answers `202` (a
public-key bearer needs fullcontrol). Clients accept an AbortSignal; HTTP failures retain status and error
code through `GizClawControlError`. Ogg downloads return a Blob.

Node Monitor generated output belongs to `gizclaw-control/generated/monitor`.
The peer wire contract remains generated once in `gizclaw/generated/peerhttp`;
the browser console consumes the control SDK rather than importing generated
clients directly.
