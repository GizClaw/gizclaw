# Desktop E2E User Story Sets

Desktop e2e covers the Wails shell that replaces the old CLI-served UI
surfaces. It uses the same setup server, resources, and committed identities as
the Go, JS, and cmd suites.

## Sets

- `shell/`: app startup, context picker, selected context persistence, and
  runtime injection.
- `admin/`: Admin resource navigation and generated-client WebRTC Admin API
  transport boundary.
- `play/`: Play workspace runtime, history replay, social/firmware resource
  scanning, and direct WebRTC RPC transport boundary.
