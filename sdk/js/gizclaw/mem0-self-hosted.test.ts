import assert from "node:assert/strict";
import test from "node:test";
import { createClient } from "./generated/adminhttp/client/index.ts";
import {
  createRuntimeProfile,
  type RuntimeProfileMem0SelfHostedConnection,
  type RuntimeProfileUpsert,
} from "./admin.ts";

test("Admin SDK sends self-hosted Mem0 profiles with optional authentication", async () => {
  for (const apiKey of [undefined, "test-key"]) {
    const connection: RuntimeProfileMem0SelfHostedConnection = {
      type: "mem0_self_hosted",
      endpoint: "http://127.0.0.1:18000",
      ...(apiKey === undefined ? {} : { api_key: apiKey }),
    };
    const body: RuntimeProfileUpsert = {
      id: "local-memory",
      spec: {
        workflows: {},
        resources: {
          memories: {
            assistant: {
              layout_id: "assistant-memory",
              driver: "mem0",
              connection,
            },
          },
        },
      },
    };
    let captured: unknown;
    const client = createClient({
      baseUrl: "http://gizclaw",
      fetch: async (input, init) => {
        const request = new Request(input, init);
        captured = await request.json();
        return Response.json(body);
      },
    });
    const result = await createRuntimeProfile({ body, client });
    assert.equal(result.error, undefined);
    assert.deepEqual(captured, body);
    assert.deepEqual(
      result.data?.spec.resources?.memories?.assistant.connection,
      connection,
    );
  }
});
