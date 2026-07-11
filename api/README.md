# API Definitions

This directory contains the source OpenAPI specifications for GizClaw HTTP APIs.
These JSON files are the contract used by generated Go clients, server
interfaces, and shared API types under `pkgs/gizclaw/api/`.

## Layout

- `admin_http.json`, `peer_http.json`, and `desktop_service.json` define
  GizClaw HTTP API surfaces.
- `rpc/common.proto` defines shared Peer RPC envelopes, errors, and stream frames.
- `rpc/peer.proto` defines the Peer RPC request envelope and method registry.
- `rpc/payload/*.proto` defines method-specific Peer RPC payload messages by
  domain.
- `openai-compat/v1/service.json` defines the OpenAI-compatible HTTP surface.
- `types.json` collects shared schemas and exposes them through
  `#/components/schemas`.
- `type/*.json` contains reusable shared schema definitions.
- `type/server.json` contains peer-owned DTO schemas still referenced by Admin
  HTTP generation.
- `resource/*.json` contains declarative admin resource schemas used by
  `admin apply`, `admin show`, and related resource APIs.

## Generated Code

Generated Go code lives outside this directory:

- `pkgs/gizclaw/api/adminhttp/generated.go`
- `pkgs/gizclaw/api/apitypes/generated.go`
- `pkgs/gizclaw/api/openaihttp/generated.go`
- `pkgs/gizclaw/api/rpcapi/generated.go`
- `pkgs/gizclaw/api/rpcapi/payload_codec.go`
- `pkgs/gizclaw/api/rpcproto/common.pb.go`
- `pkgs/gizclaw/api/rpcproto/peer.pb.go`
- `pkgs/gizclaw/api/rpcproto/{ai,edge,enums,firmware,gameplay,social,system,workspace}.pb.go`
- `pkgs/gizclaw/api/peerhttp/generated.go`

`rpcproto` is generated directly from `api/rpc/common.proto`,
`api/rpc/peer.proto`, and the split `api/rpc/payload/*.proto` files. The
`rpcapi` package is the committed Go wrapper surface on top of those protobuf
payload descriptors; when changing Peer RPC methods or payload messages, update
`rpcapi/generated.go` and `rpcapi/payload_codec.go` in the same change and
verify them with `go test ./pkgs/gizclaw/api/rpcapi`. Do not generate Go RPC
payload DTOs by converting protobuf schemas to JSON Schema or OpenAPI first;
use the protoc-generated `rpcproto` Go messages directly. Edge route RPC
payloads such as `server.peer.lookup`, `server.peer.assign`, and
`server.route.resolve` use those `rpcproto` messages through `rpcapi` aliases;
do not add JSON-schema DTOs for those payloads under `api/type/server.json`.

Current generated TypeScript SDK code lives under `sdk/js/gizclaw/`:

- `sdk/js/gizclaw/generated/adminhttp`
- `sdk/js/gizclaw/generated/rpc`
- `sdk/js/gizclaw/generated/peerhttp`

The old browser `client_service` API and CLI-served UI TypeScript clients were
removed as part of the desktop clean break. Desktop UI code should consume the
generated clients through `@gizclaw/gizclaw` WebRTC transports instead of
reintroducing old CLI-served package boundaries.

Do not edit generated files by hand. Change the source schema in `api/`, then
regenerate the corresponding Go and/or TypeScript package.

Common commands:

```sh
go generate ./pkgs/gizclaw/api/adminhttp
go generate ./pkgs/gizclaw/api/apitypes
go generate ./pkgs/gizclaw/api/openaihttp
go generate ./pkgs/gizclaw/api/rpcproto
go generate ./pkgs/gizclaw/api/peerhttp
go test ./pkgs/gizclaw/api/rpcapi
```

Regenerate TypeScript SDK packages with:

```sh
npm --prefix sdk/js run gen:sdk
```

When in doubt, regenerate all API packages:

```sh
go generate ./pkgs/gizclaw/api/...
```

## Maintenance Guidelines

- Treat files in `api/` as public contracts. Keep changes small, explicit, and
  covered by tests at the service or CLI boundary.
- Prefer adding reusable schemas under `type/` or `resource/` and referencing
  them from top-level OpenAPI documents instead of duplicating inline schemas.
- Keep schema names, discriminator values, and path operation IDs stable unless
  the caller-facing contract is intentionally changing.
- When adding or changing an endpoint, update the OpenAPI document, regenerate
  Go code, implement the strict server interface, and add tests for both success
  and user-visible error paths.
- When changing declarative admin resources, verify `resourcemanager` behavior
  and CLI stories under `tests/gizclaw-e2e/cmd/` when applicable.
- Run focused tests for the touched API surface and coverage-sensitive packages.
  For broader API changes, prefer:

```sh
go test ./pkgs/gizclaw/... ./cmd/... -count=1
```
