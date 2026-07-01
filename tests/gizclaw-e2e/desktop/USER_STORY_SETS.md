# Desktop E2E User Story Sets

Desktop e2e covers the Wails shell that replaces the old CLI-served UI
surfaces. It uses the same setup server, resources, and committed identities as
the Go, JS, and cmd suites.

## Sets

- `shell/`: app startup, context picker, selected context persistence, and
  runtime injection.
- `admin/`: populated when the Admin view is rewritten into the desktop app.
- `play/`: populated when the Play view is rewritten into the desktop app.
