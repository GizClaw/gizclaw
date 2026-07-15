# GizClaw Desktop

`apps/wails` is a Pod-oriented desktop control plane. The Wails window manages
local and remote server environments; Admin and Play remain browser applications
served on loopback-only random ports and opened in the system browser.

## Pod storage

Pods live under `os.UserConfigDir()/GizClaw/pods/<id>/` by default:

```text
<pod>/
├── pod.json
├── workspace/                 # local Pods only
│   └── config.yaml
├── admin_context/<server-id>/ # only where Admin is configured
│   └── config.yaml
└── client_context/            # only where Play is configured
    └── config.yaml
```

`pod.json` is the source of truth. Projection files are rebuilt after each
manifest update. Pod directories are mode `0700`; manifests, workspace config,
and Context config files are atomically written with mode `0600`.

A local Pod has one `local_server` with a stable port. The Server listens on
`0.0.0.0:<port>` for LAN access while its local Admin and Client Contexts use
`127.0.0.1:<port>`. A remote Pod has `remote_servers` plus one
`remote_access_point`. Admin identity is per Server; Client identity is per Pod.

Set `GIZCLAW_DESKTOP_CONFIG_HOME` to isolate storage in development or tests.
Development runs may set `GIZCLAW_DESKTOP_SERVER_EXECUTABLE` or use `gizclaw`
from `PATH`.

Packaged macOS builds use `scripts/package-darwin.sh`. It runs the production
Wails build and compiles `cmd/gizclaw` into
`GizClaw.app/Contents/Resources/gizclaw`; the local lifecycle manager resolves
that bundled companion before considering development fallbacks. A raw
`wails build` is suitable for UI validation but is not the distribution package
for local Server support.

## Runtime boundaries

- The Wails bridge returns only configured/missing state; persisted private keys
  never appear in Pod responses.
- Endpoint health uses bounded native `GET /server-info` probes without
  credentials.
- Each Pod reuses at most one Admin listener and one Play listener, both bound
  to `127.0.0.1:0`.
- Every browser launch uses a fresh, single-use runtime handoff. Private keys are
  not placed in URLs, browser storage, static assets, or logs.
- Closing the window hides it. The system tray contains only Open Window,
  per-Pod Open Pod…, and Quit navigation.

## Development

```sh
npm ci
npm --prefix apps/wails/frontend run build
npm --prefix apps/wails/frontend test
npm --prefix apps/wails/frontend run test:e2e

cd apps/wails
go test ./...
./scripts/package-darwin.sh
```

The desktop OpenAPI source is `api/http/desktop.json`. Regenerate its committed
TypeScript surface through `npm --prefix sdk/js run gen:sdk`.
