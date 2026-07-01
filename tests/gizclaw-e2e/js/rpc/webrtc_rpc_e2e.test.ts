import assert from "node:assert/strict";
import path from "node:path";

import { createPeerRPCClient } from "@gizclaw/gizclaw/rpc";
import { assertSetupServerAvailable, closePeerConnection, connectSetupPeer, loadIdentity, repoRoot } from "../common/webrtc.ts";

const identityDir = process.env.GIZCLAW_E2E_JS_IDENTITY_DIR ?? path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/peer");

async function main(): Promise<void> {
  const identity = await loadIdentity(identityDir);
  await assertSetupServerAvailable(identity.endpoint);

  const pc = await connectSetupPeer(identityDir);
  try {
    const rpc = createPeerRPCClient(pc as unknown as RTCPeerConnection, {
      requestTimeoutMs: 10_000,
    });
    const result = await rpc.call("all.ping", {
      client_send_time: Date.now(),
    });

    assert.equal(typeof result.server_time, "number");
    assert.ok(result.server_time > 0);
  } finally {
    closePeerConnection(pc);
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}

main().then(
  () => {
    console.log("ok - Node WebRTC SDK connects to setup server and runs all.ping");
    process.exit(0);
  },
  (err: unknown) => {
    console.error(err);
    process.exit(1);
  },
);
