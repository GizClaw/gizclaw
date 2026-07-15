# Desktop E2E User Story Sets

Desktop e2e covers the Pod-oriented Wails control plane and its separate
browser-served Admin and Play surfaces.

## Sets

- `shell/`: Pod storage, card/detail UI, endpoint health, process lifecycle, and
  single-use browser Runtime handoff.
- `admin/`: Admin resource navigation and generated-client WebRTC Admin API
  transport boundary.
- `play/`: Play workspace runtime, history replay, social/firmware resource
  scanning, and direct WebRTC RPC transport boundary.
