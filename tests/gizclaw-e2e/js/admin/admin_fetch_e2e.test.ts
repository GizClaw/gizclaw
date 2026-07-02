import assert from "node:assert/strict";
import path from "node:path";

import { createAdminAPIClient, listPeers } from "@gizclaw/gizclaw/admin";
import { assertSetupServerAvailable, closePeerConnection, connectSetupPeer, loadIdentity, repoRoot } from "../common/webrtc.ts";

const identityDir = process.env.GIZCLAW_E2E_JS_ADMIN_IDENTITY_DIR ?? path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/admin");

async function main(): Promise<void> {
  const identity = await loadIdentity(identityDir);
  await assertSetupServerAvailable(identity.endpoint);

  const pc = await connectSetupPeer(identityDir);
  try {
    const client = createAdminAPIClient(pc as unknown as RTCPeerConnection, { requestTimeoutMs: 10_000 });
    const response = await listPeers({
      client,
      query: { limit: 5 },
      throwOnError: true,
    });
    assert.equal(Array.isArray(response.data.items), true);
  } finally {
    closePeerConnection(pc);
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}

main().then(
  () => {
    console.log("ok - Node WebRTC SDK fetches Admin API over the admin service channel");
    process.exit(0);
  },
  (err: unknown) => {
    console.error(err);
    process.exit(1);
  },
);
