# API Definitions

This directory contains the source OpenAPI specifications for GizClaw HTTP APIs.
These JSON files are the contract used by generated Go clients, server
interfaces, and shared API types under `pkg/gizclaw/api/`.

## Layout

- `admin_service.json`, `gear_service.json`, `openai_service.json`, `server_public.json`,
  and `rpc.json` define API surfaces or shared
  protocol documents.
- `types.json` collects shared schemas and exposes them through
  `#/components/schemas`.
- `type/*.json` contains reusable shared schema definitions.
- `rpc/*.json` contains reusable RPC method schema definitions.
- `resource/*.json` contains declarative admin resource schemas used by
  `admin apply`, `admin show`, and related resource APIs.

## Generated Code

Generated Go code lives outside this directory:

- `pkg/gizclaw/api/adminservice/generated.go`
- `pkg/gizclaw/api/apitypes/generated.go`
- `pkg/gizclaw/api/gearservice/generated.go`
- `pkg/gizclaw/api/openaiservice/generated.go`
- `pkg/gizclaw/api/rpcapi/generated.go`
- `pkg/gizclaw/api/serverpublic/generated.go`

Do not edit generated files by hand. Change the source schema in `api/`, then
regenerate the corresponding Go package.

Common commands:

```sh
go generate ./pkg/gizclaw/api/adminservice
go generate ./pkg/gizclaw/api/apitypes
go generate ./pkg/gizclaw/api/gearservice
go generate ./pkg/gizclaw/api/openaiservice
go generate ./pkg/gizclaw/api/rpcapi
go generate ./pkg/gizclaw/api/serverpublic
```

When in doubt, regenerate all API packages:

```sh
go generate ./pkg/gizclaw/api/...
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
  and CLI stories under `test/gizclaw-e2e/cmd/` when applicable.
- Run focused tests for the touched API surface and coverage-sensitive packages.
  For broader API changes, prefer:

```sh
go test ./pkg/gizclaw/... ./cmd/... -count=1
```
