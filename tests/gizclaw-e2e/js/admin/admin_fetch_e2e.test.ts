import assert from "node:assert/strict";
import path from "node:path";

import {
  applyResource,
  createAdminAPIClient,
  getResource,
  listPeers,
  listWorkflows,
  type ResourceWritable,
} from "@gizclaw/gizclaw/admin";
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

    const workflows = await listWorkflows({ client, throwOnError: true });
    const workflowName = workflows.data.items[0]?.name;
    assert.notEqual(workflowName, undefined, "setup server must contain a Workflow fixture");
    const current = await getResource({
      client,
      path: { kind: "Workflow", name: workflowName! },
      throwOnError: true,
    });
    type MutableAdminResource = {
      metadata: { annotations?: Record<string, string> };
    } & Record<string, unknown>;
    const original = structuredClone(current.data) as unknown as MutableAdminResource;
    const large = structuredClone(original);
    large.metadata.annotations = {
      ...large.metadata.annotations,
      "gizclaw.io/service-stream-e2e": "x".repeat(70 * 1024),
    };
    assert.equal(JSON.stringify(large).length > 64 * 1024, true);
    let changed = false;
    try {
      await applyResource({ body: large as unknown as ResourceWritable, client, throwOnError: true });
      changed = true;
      const unchanged = await applyResource({
        body: large as unknown as ResourceWritable,
        client,
        throwOnError: true,
      });
      assert.equal(unchanged.data.action, "unchanged");
    } finally {
      if (changed) {
        await applyResource({ body: original as unknown as ResourceWritable, client, throwOnError: true });
      }
    }
  } finally {
    closePeerConnection(pc);
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}

main().then(
  () => {
    console.log("ok - Node WebRTC SDK fetches Admin API over the admin HTTP service channel");
    process.exit(0);
  },
  (err: unknown) => {
    console.error(err);
    process.exit(1);
  },
);
