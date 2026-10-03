import assert from "node:assert/strict";
import test from "node:test";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { createClient } from "./generated/adminhttp/client/index.ts";
import {
  createRuntimeProfile,
  type RuntimeProfileQuota,
  type RuntimeProfileSpec,
  type RuntimeProfileUpsert,
} from "./admin.ts";

const profiles: RuntimeProfileSpec[] = [
  {},
  { quota: null },
  { quota: { type: "unlimited" } },
  {
    quota: {
      type: "custom",
      endpoint: "https://quota.example.test/v1/quota",
      api_key: "test-key",
    },
  },
];

// @ts-expect-error An explicitly supplied empty policy is invalid.
const empty: RuntimeProfileQuota = {};
// @ts-expect-error Only unlimited and custom are supported.
const unknown: RuntimeProfileQuota = { type: "rate_limit" };
// @ts-expect-error Custom policy requires its API key.
const incomplete: RuntimeProfileQuota = {
  type: "custom",
  endpoint: "https://quota.example.test/v1/quota",
};
void [empty, unknown, incomplete];

test("Admin quota policy assignments compile against generated SDK types", () => {
  const program = ts.createProgram([fileURLToPath(import.meta.url)], {
    allowImportingTsExtensions: true,
    allowJs: true,
    module: ts.ModuleKind.NodeNext,
    moduleResolution: ts.ModuleResolutionKind.NodeNext,
    noEmit: true,
    skipLibCheck: true,
    strict: true,
    target: ts.ScriptTarget.ES2024,
    types: ["node"],
  });
  const diagnostics = ts.getPreEmitDiagnostics(program);
  assert.equal(
    diagnostics.length,
    0,
    ts.formatDiagnosticsWithColorAndContext(diagnostics, {
      getCanonicalFileName: (name) => name,
      getCurrentDirectory: () => process.cwd(),
      getNewLine: () => "\n",
    }),
  );
});

test("Admin SDK preserves omitted, null, unlimited and custom quota policies", async () => {
  for (const spec of profiles) {
    const body: RuntimeProfileUpsert = { id: "quota-policy", spec };
    let captured: unknown;
    const client = createClient({
      baseUrl: "http://gizclaw",
      fetch: async (input, init) => {
        captured = await new Request(input, init).json();
        return Response.json(body);
      },
    });
    const result = await createRuntimeProfile({ body, client });
    assert.equal(result.error, undefined);
    assert.deepEqual(captured, body);
    assert.deepEqual(result.data?.spec, spec);
  }
});
