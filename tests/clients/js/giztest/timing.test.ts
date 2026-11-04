import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { loadDocument } from "./document.ts";
import { runDocuments } from "./runner.ts";

const document = `# User Story:
# As a Giztest SDK tester,
# I want to accept shared load timing fields,
# So that SDK checks can validate shared documents without simulating load.
version: gizclaw.test/v1alpha1
name: timing
clients:
  peer: {identity: ephemeral, connection: webrtc, access_point: localhost:9820}
variables: {}
steps:
  - id: ping
    client: peer
    rpc: {method: all.ping, request: {}}
`;

test("timing fields are accepted and validated before being ignored", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "giztest-timing-"));
  const file = path.join(directory, "timing.giztest.yaml");
  try {
    for (const fields of [
      "start_jitter: 30s\nstagger: 2s\nstep_jitter: 3s\nseed: 0\n",
      "start_jitter: '0'\nstagger: 1h2m3.5s\nseed: 9007199254740991\n",
      "start_jitter: 9223372036854775807ns\n",
    ]) {
      await writeFile(file, document + fields);
      assert.equal((await loadDocument(file)).name, "timing");
    }
    for (const fields of [
      "start_jitter: -1s\n",
      "stagger: nonsense\n",
      "step_jitter: 0..3s\n",
      "start_jitter: ''\n",
      "start_jitter: 0\n",
      "start_jitter: null\n",
      "step_jitter: {min: 0s, max: 3s}\n",
      "seed: -1\n",
      "seed: 1.5\n",
      "seed: '1'\n",
      "seed: null\n",
      "seed: 9007199254740992\n",
      "start_jitter: 9223372036854775808ns\n",
      "repeat: 3\nstagger: 4611686018427387904ns\n",
      "repeat: 2\nstagger: 9223372036854775807ns\nstart_jitter: 2ns\n",
    ]) {
      await writeFile(file, document + fields);
      await assert.rejects(loadDocument(file), /duration|seed/u, fields);
    }
    await writeFile(
      file,
      document.replace(
        "    rpc: {method: all.ping, request: {}}",
        "    barrier: {}",
      ) + "step_jitter: 3s\n",
    );
    await assert.rejects(loadDocument(file), /barrier cannot be combined/u);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("SDK reports identify ignored scheduling", async () => {
  const report = await runDocuments([], { parallel: 1 });
  assert.equal(report.timing_mode, "ignored");
});
