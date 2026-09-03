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

`serveGiznetWebRTCRPC(pc, handlers)` answers the `client.*` RPCs a server
initiates. `GizClawPeerRPCHandlers` covers `client.info.get`,
`client.identifiers.get` and the seven `client.device.*` and `client.wifi.*`
methods; an omitted handler answers `METHOD_NOT_FOUND`, which the server maps
to `501 DEVICE_UNSUPPORTED`. A handler can throw `GizClawDeviceControlError` to
answer one specific RPC error code. Handlers can also be passed through the
`peerRPCHandlers` connect option so they are installed before signaling.

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

Both packages are published to GitHub Packages by `.github/workflows/js-sdk-release.yml`. `sdk/js/scripts/check-package-release.mjs --package sdk/js/<name>` verifies each package separately:

- when release paths change (files inside the package directory other than the `package.json` version, `*.test.ts`, and `tsconfig.json`, plus `scripts/prepare-published-sdk.mjs`), the version must be higher than at the base commit;
- when no release path changes, the version must not change;
- the workspace version in `package-lock.json` must match the manifest.

Changing the public surface of `@gizclaw/gizclaw` (for example adding an export) requires bumping its version; a change limited to `gizclaw-control` does not trigger an `@gizclaw/gizclaw` release. `prepare-published-sdk.mjs <package>` runs after `tsc`, copies generated Protobuf JavaScript when the package has it, and rewrites `.ts` imports in `.d.ts` files to `.js`.
