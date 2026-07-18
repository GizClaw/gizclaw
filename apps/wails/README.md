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
├── admin_context/<context-id>/ # projected Admin contexts
│   └── config.yaml
└── client_context/            # generated desktop-local Play identity
    └── config.yaml
```

`pod.json` is the source of truth. Projection files are rebuilt after each
manifest update. Pod directories are mode `0700`; manifests, workspace config,
and Context config files are atomically written with mode `0600`.

The same config root contains a private, editable `bootstrap.env` dotenv file.
It stores provider values used only while creating future local Pods. Desktop
offers both a human-readable form and a raw dotenv editor for this file.
Desktop-saved values override process environment values; resource-declared
defaults are used last.

A local Pod has one `local_server` with a stable port. The Server listens on
`0.0.0.0:<port>` for LAN access while its local Admin and Client Contexts use
`127.0.0.1:<port>`. The generated Server workspace publishes a current LAN
candidate when one is available; that address is not persisted in `pod.json`.
A local Pod automatically generates its Server identity, Admin identity, and
desktop-local Play identity. Existing Pods missing these identities are filled
on desktop bootstrap. The share QR contains only the display name, selected LAN
endpoint, and Server public key; a scanning client generates its own identity.
A new local Pod is returned as soon as its manifest and projections are
persisted. The response carries an `initializing` state while a cancellable
background task starts the Server, applies the embedded deploy-derived catalog,
syncs Volc voices, and uploads all Workflow and PetDef assets. A successful task
clears the state; a failed task stops the process and persists its redacted
error so the Pod remains visible and deletable. Desktop startup removes a Pod
left actively initializing after an interrupted creation, while failed Pods
remain visible. Successful Pods are never reconciled or bootstrapped again
during start, restart, or app upgrade.
A remote Pod has one `remote_access_point` and zero or more
`remote_servers`; Servers may be added after the Pod is created. Each Server's
Admin private key is supplied by the user and stored write-only; omitting it
during an edit preserves the existing value. The desktop Play identity is
generated per Pod. Pod and Server IDs are generated as internal identifiers and
are not creation-form fields.

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

- Persisted Admin and Client private keys never appear in Wails bridge responses.
  Public identity halves may be returned for QR identity pinning and remote
  Admin setup.
- The trusted Desktop Renderer receives editable `bootstrap.env` content and
  saved values so its form and dotenv views can be prefilled. Values sourced
  only from the process environment or resource defaults are not returned.
- Endpoint health uses bounded native `GET /server-info` probes without
  credentials.
- Each Pod reuses at most one Admin listener and one Play listener, both bound
  to `127.0.0.1:0`.
- Every Admin or Play listener uses one random runtime token that remains in the
  local URL query and can be reused across opens and page refreshes until that
  listener closes. Runtime private keys remain in Desktop memory and are not placed
  in the URL, browser storage, static assets, or logs.
- The frameless shell provides native-runtime hide, minimise, and maximise
  controls. Closing the window hides it while Server and browser listeners keep
  running.
- The system tray uses a visible platform icon and contains only Open Window,
  per-Pod Open Pod…, and Quit navigation. Quit is the explicit process exit.

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
