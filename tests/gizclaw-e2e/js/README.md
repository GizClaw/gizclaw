# GizClaw JS E2E

This directory is reserved for JavaScript and TypeScript e2e suites that use the
shared `setup/` server and `testdata/` fixtures.

Expected suites:

- `admin/`: generated Admin API client plus WebRTC fetch transport
- `rpc/`: `@gizclaw/webrtc` Node runtime RPC coverage
- `chat/`: chat/workspace flows over WebRTC RPC
- `social/`: social flows over WebRTC RPC

The implementation belongs to the WebRTC SDK and desktop follow-up issues.
