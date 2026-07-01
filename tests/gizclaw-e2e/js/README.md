# GizClaw JS E2E

This directory is reserved for JavaScript and TypeScript e2e suites that use the
shared `setup/` server and `testdata/` fixtures.

Expected suites:

- `admin/`: generated Admin API client plus WebRTC fetch transport
- `rpc/`: `@gizclaw/webrtc` Node runtime RPC coverage
- `chat/`: chat/workspace flows over WebRTC RPC
- `social/`: social flows over WebRTC RPC

Current coverage:

- `rpc/webrtc_rpc_e2e.test.ts` uses the shared setup server and
  `testdata/identities/peer` to establish a real server-public WebRTC
  connection, then runs `all.ping` over the RPC service data channel.

Run after `setup/start-server.sh` and `setup/reset_data.sh reset`:

```bash
cd tests/gizclaw-e2e/js
npm run test:rpc
```
