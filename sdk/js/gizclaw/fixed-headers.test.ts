import assert from "node:assert/strict";
import test from "node:test";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { createClient } from "./generated/adminhttp/client/index.ts";
import {
  createCredential,
  createModel,
  type CredentialUpsert,
  type ModelRequestSource,
  type ModelUpsert,
} from "./admin.ts";

const credential: CredentialUpsert = {
  id: "fixed-header-fixture",
  provider: "openai",
  body: {
    api_key: "local-upstream-fixture",
    headers: { "X-GizProxy-Authorization": "Bearer local-proxy-fixture" },
  },
};
const model: ModelUpsert = {
  id: "fixed-header-fixture",
  kind: "llm",
  source: "manual",
  provider: { kind: "openai-tenant", id: "proxy-fixture" },
  provider_data: {
    upstream_model: "chat-fixture",
    request_bindings: [
      {
        target: { kind: "http_header", name: "X-GizProxy-Upstream-Host" },
        source: { constant: "vendor.example:30100" },
      },
      {
        target: { kind: "http_header", name: "X-AI-UID" },
        source: { peer_runtime_kv: { key: "lingxi", pointer: "/device_id" } },
      },
    ],
  },
};
// @ts-expect-error Fixed header values must be strings.
const invalid: ModelRequestSource = { constant: 42 };
void invalid;

test("fixed header configuration compiles against generated Admin types", () => {
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
  assert.deepEqual(ts.getPreEmitDiagnostics(program), []);
});

test("Admin SDK preserves fixed headers and Runtime binding branches", async () => {
  const captured: unknown[] = [];
  const client = createClient({
    baseUrl: "http://gizclaw",
    fetch: async (input, init) => {
      const body: unknown = await new Request(input, init).json();
      captured.push(body);
      return Response.json(body);
    },
  });
  const keyResult = await createCredential({ body: credential, client });
  const modelResult = await createModel({ body: model, client });
  assert.equal(keyResult.error, undefined);
  assert.equal(modelResult.error, undefined);
  assert.deepEqual(captured, [credential, model]);
});
