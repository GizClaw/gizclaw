import assert from "node:assert/strict";
import { test } from "node:test";

import { formatRoute, parseRoute, type ConsoleRoute } from "../src/routes.ts";

test("every route survives a hash round trip", () => {
  const routes: ConsoleRoute[] = [
    { page: "overview" },
    { page: "server", id: "edge-bj" },
    { page: "server", id: "edge/上海 1" },
    { page: "peers" },
    { page: "peer", publicKey: "7kZx2V9pQwErTyUi1oPaSdFgHjKlZxCvBnM3qW5eR8tY" },
    { page: "logs" },
    {
      page: "logs",
      query: "peer_public_key:abc error_code:ASR_TIMEOUT -level:info",
    },
  ];
  for (const route of routes) {
    assert.deepEqual(parseRoute(formatRoute(route)), route);
  }
});

test("hashes match the console router", () => {
  assert.equal(formatRoute({ page: "overview" }), "#/");
  assert.equal(formatRoute({ page: "peers" }), "#/peers");
  assert.equal(formatRoute({ page: "logs", query: "a b" }), "#/logs?q=a%20b");
  assert.equal(formatRoute({ page: "server", id: "a/b" }), "#/server/a%2Fb");
});

test("unknown or empty hashes fall back to the overview", () => {
  for (const hash of ["", "#", "#/unknown", "#/server/", "#/peer/"]) {
    assert.deepEqual(parseRoute(hash), { page: "overview" });
  }
  assert.deepEqual(parseRoute("#/logs?q="), { page: "logs" });
});
