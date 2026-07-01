import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";

import { WebRTCRPCClient } from "@gizclaw/webrtc";
import { closePeerConnection, connectSetupPeer, loadIdentity, repoRoot, serverAvailable } from "../common/webrtc.ts";

const identityDir = process.env.GIZCLAW_E2E_JS_IDENTITY_DIR ?? path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/peer");

test("Node WebRTC SDK connects to setup server and runs all.ping", async (t) => {
  const identity = await loadIdentity(identityDir);
  if (!(await serverAvailable(identity.endpoint))) {
    t.skip(`e2e setup server is not available at ${identity.endpoint}`);
    return;
  }

  const pc = await connectSetupPeer(identityDir);
  try {
    const rpc = new WebRTCRPCClient(pc as unknown as RTCPeerConnection, {
      createID: () => "js.all.ping",
      requestTimeoutMs: 10_000,
    });
    const result = await rpc.call<{ server_time: number }>("all.ping", {
      client_send_time: Date.now(),
    });

    assert.equal(typeof result.server_time, "number");
    assert.ok(result.server_time > 0);
  } finally {
    closePeerConnection(pc);
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
});
