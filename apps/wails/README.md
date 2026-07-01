# GizClaw Desktop

`apps/wails` is the new GizClaw desktop shell. It replaces the old
CLI-hosted Admin UI and Play UI surfaces with a Wails app that stores local
contexts outside the WebView and talks to GizClaw servers from the frontend over
WebRTC.

## Runtime Boundary

- The Go backend manages local context files and selected UI state.
- The Go backend does not call GizClaw server APIs or proxy RPC/API requests.
- The frontend receives the selected context, signaling URL, and private key
  material from Wails at runtime and keeps it in memory.
- Browser persistence such as `localStorage` and IndexedDB must not be used for
  private key material.

## Local State

By default the app stores state under the OS user config directory:

```text
gizclaw-desktop/
├── contexts/
│   ├── current -> <context-name>
│   └── <context-name>/
│       ├── config.yaml
│       └── identity.key
└── state.json
```

For tests and local development, set `GIZCLAW_DESKTOP_CONFIG_HOME` to isolate
this directory.

## Development

```sh
cd apps/wails
go mod tidy
cd ../..
npm install
npm --prefix apps/wails/frontend run build
go test ./apps/wails/...
```

The shell currently provides context selection plus Admin and Play placeholder
views. The actual Admin and Play experiences are rewritten in the follow-up
issues under the #109 desktop plan.
