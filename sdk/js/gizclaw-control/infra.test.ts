import assert from "node:assert/strict";
import test from "node:test";
import { createClient } from "./generated/infra/client/index.ts";
import {
  preflightInfraFlows,
  readInfraFlows,
  type Environment,
  type Page,
} from "./generated/infra/index.ts";

const emptyPage: Page = {
  cursor: "",
  reset: false,
  server_time: 5000,
  frames: [],
  limits: {
    window_ms: 5000,
    max_flows: 64,
    retention_frames: 120,
    max_read_frames: 3,
  },
};

test("generated infrastructure reads and preflights need no credentials", async () => {
  const requests: Request[] = [];
  const client = createClient({
    baseUrl: "https://ap.example",
    auth: () => assert.fail("public operation consulted credential provider"),
    fetch: async (input) => {
      const request = new Request(input);
      requests.push(request);
      assert.equal(request.headers.has("Authorization"), false);
      assert.equal(new URL(request.url).pathname, "/gizclaw/v1/infra/flows");
      if (request.method === "OPTIONS") {
        return new Response(null, { status: 204 });
      }
      assert.equal(request.method, "GET");
      return Response.json(emptyPage);
    },
  });
  for (const environment of ["dev", "e2e", "prod", "trial"] as const) {
    const response = await readInfraFlows({
      client,
      query: { environment },
    });
    assert.deepEqual(response.data, emptyPage);
    assert.equal(
      new URL(requests.at(-1)!.url).searchParams.get("environment"),
      environment,
    );
  }
  await readInfraFlows({
    client,
    query: { environment: "prod", cursor: "5000" },
  });
  assert.equal(
    new URL(requests.at(-1)!.url).searchParams.get("cursor"),
    "5000",
  );
  const preflight = await preflightInfraFlows({ client });
  assert.equal(preflight.response.status, 204);
});

test("generated public read returns bounded-query and unavailable errors", async () => {
  for (const [status, error] of [
    [400, "INVALID_FLOW_QUERY"],
    [503, "FLOW_FEED_DISABLED"],
    [503, "FLOW_FEED_UNAVAILABLE"],
  ] as const) {
    const client = createClient({
      baseUrl: "https://ap.example",
      fetch: async () => Response.json({ error }, { status }),
    });
    const environment: Environment = "prod";
    const result = await readInfraFlows({ client, query: { environment } });
    assert.equal(result.response.status, status);
    assert.deepEqual(result.error, { error });
  }
});
