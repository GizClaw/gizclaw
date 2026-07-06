# GizClaw JS E2E

This directory is reserved for JavaScript and TypeScript e2e suites that use the
Docker e2e server and `testdata/` fixtures.

Expected suites:

- `admin/`: generated Admin API client plus WebRTC fetch transport
- `rpc/`: `@gizclaw/gizclaw` Node runtime RPC coverage
- `chat/`: chat/workspace flows over WebRTC RPC
- `social/`: social flows over WebRTC RPC

Current coverage:

- `admin/admin_fetch_e2e.test.ts` uses the shared setup server and
  `testdata/identities/admin` to establish a real server-public WebRTC
  connection, then fetches the Admin HTTP API through the Admin service data
  channel.
- `rpc/webrtc_rpc_e2e.test.ts` uses the shared setup server and
  `testdata/identities/peer` to establish a real server-public WebRTC
  connection, then runs `all.ping` over the RPC service data channel.

Run through the default Docker e2e gate:

```bash
bash tests/gizclaw-e2e/run_tests.sh
```

For focused manual runs, start the Docker Compose stack from the repository
root, export the generated identity directories from
`tests/gizclaw-e2e/testdata/docker/<project>/docker.env`, then run:

```bash
cd tests/gizclaw-e2e/js
npm run test:admin
npm run test:rpc
```
