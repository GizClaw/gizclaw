# GizClaw Desktop E2E

Desktop e2e is split by ownership:

- `shell/`: Pod cards, manifest/context projection, browser handoff, build and
  startup smoke checks;
- `admin/`: the browser-served Admin application and WebRTC Admin transport;
- `play/`: the browser-served Play application and direct peer RPC transport.

The shell uses a mock Wails Pod bridge for Playwright UI behavior. Go tests cover
private filesystem projection, health probes, local process management, and the
loopback HTTP handoff boundary.
