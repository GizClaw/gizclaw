# API Definitions

This directory contains the source OpenAPI specifications for GizClaw HTTP APIs.
These JSON files are the contract used by generated Go clients, server
interfaces, and shared API types under `pkgs/gizclaw/api/`.

## Layout

- `admin_service.json`, `server_public.json`, `desktop_service.json`, and
  `rpc.json` define GizClaw API surfaces or shared protocol documents.
- `openai-compat/v1/service.json` defines the OpenAI-compatible HTTP surface.
- `types.json` collects shared schemas and exposes them through
  `#/components/schemas`.
- `type/*.json` contains reusable shared schema definitions.
- `rpc/*.json` contains reusable RPC method schema definitions.
- `resource/*.json` contains declarative admin resource schemas used by
  `admin apply`, `admin show`, and related resource APIs.

## Generated Code

Generated Go code lives outside this directory:

- `pkgs/gizclaw/api/adminservice/generated.go`
- `pkgs/gizclaw/api/apitypes/generated.go`
- `pkgs/gizclaw/api/openaiservice/generated.go`
- `pkgs/gizclaw/api/rpcapi/generated.go`
- `pkgs/gizclaw/api/serverpublic/generated.go`

Current generated TypeScript SDK code lives under `sdk/js/gizclaw/`:

- `sdk/js/gizclaw/generated/adminservice`
- `sdk/js/gizclaw/generated/rpc`
- `sdk/js/gizclaw/generated/serverpublic`

The old browser `client_service` API and CLI-served UI TypeScript clients were
removed as part of the desktop clean break. Desktop UI code should consume the
generated clients through `@gizclaw/gizclaw` WebRTC transports instead of
reintroducing old CLI-served package boundaries.

Do not edit generated files by hand. Change the source schema in `api/`, then
regenerate the corresponding Go and/or TypeScript package.

Common commands:

```sh
go generate ./pkgs/gizclaw/api/adminservice
go generate ./pkgs/gizclaw/api/apitypes
go generate ./pkgs/gizclaw/api/openaiservice
go generate ./pkgs/gizclaw/api/rpcapi
go generate ./pkgs/gizclaw/api/serverpublic
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
