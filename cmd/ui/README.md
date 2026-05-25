# GizClaw UI

This directory contains the frontend source for the embedded `admin` and `play` web UIs.

- `admin`: admin UI entrypoint
- `play`: play UI entrypoint
- `*/components`: UI-owned components for each app

Generated TypeScript SDKs live in the repository-level npm workspace under `../../js/packages`:

- `../../js/packages/adminservice`: generated SDK for `api/admin_service.json`
- `../../js/packages/serverpublic`: generated SDK for `api/server_public.json`
- `../../js/packages/gearservice`: generated SDK for `api/gear_service.json`

Each app owns its own generated assets and embedded Go package:

- `go generate ./cmd/ui/admin`
- `go generate ./cmd/ui/play`

OpenAPI TypeScript SDKs are generated with:

- `npm run gen:sdk`

`npm run gen:sdk` delegates to the `@gizclaw/codegen` workspace in `../../js`. The generated SDKs under `../../js/packages/*` are committed, so UI builds do not need to re-run SDK generation unless those specs change.
