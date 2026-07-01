import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";

import { createAdminAPIFetch } from "@gizclaw/webrtc";
import { closePeerConnection, connectSetupPeer, loadIdentity, repoRoot, serverAvailable } from "../common/webrtc.ts";

const identityDir = process.env.GIZCLAW_E2E_JS_ADMIN_IDENTITY_DIR ?? path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/admin");

test("Node WebRTC SDK fetches Admin API over the admin service channel", async (t) => {
  const identity = await loadIdentity(identityDir);
  if (!(await serverAvailable(identity.endpoint))) {
    t.skip(`e2e setup server is not available at ${identity.endpoint}`);
    return;
  }

  const pc = await connectSetupPeer(identityDir);
  try {
    const adminFetch = createAdminAPIFetch(pc as unknown as RTCPeerConnection, { requestTimeoutMs: 10_000 });
    const response = await adminFetch("http://gizclaw/peers?limit=5");
    assert.equal(response.ok, true);
    const body = (await response.json()) as { items?: unknown[] };
    assert.equal(Array.isArray(body.items), true);
  } finally {
    closePeerConnection(pc);
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
});
