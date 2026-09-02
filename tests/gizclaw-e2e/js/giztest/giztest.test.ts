import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";

import { assertValue, jsonEqual, jsonPointer, safeError } from "./assert.ts";
import { httpBaseURL } from "./client.ts";
import {
  collectReferences,
  discover,
  loadDocument,
  loadDocuments,
  parseDuration,
  stepOperation,
} from "./document.ts";
import { generateValue, Variables } from "./variables.ts";

const scenarioRoot = path.resolve(import.meta.dirname, "../../giztest");

test("jsonPointer resolves objects, arrays and escapes", () => {
  const input = {
    "a/b": 1,
    "n~m": 2,
    items: [{ value: "first" }, { value: "second" }],
    nested: { empty: "" },
  };
  assert.deepEqual(jsonPointer(input, ""), { found: true, value: input });
  assert.deepEqual(jsonPointer(input, "/items/1/value"), {
    found: true,
    value: "second",
  });
  assert.deepEqual(jsonPointer(input, "/a~1b"), { found: true, value: 1 });
  assert.deepEqual(jsonPointer(input, "/n~0m"), { found: true, value: 2 });
  assert.equal(jsonPointer(input, "/items/2").found, false);
  assert.equal(jsonPointer(input, "/items/-1").found, false);
  assert.equal(jsonPointer(input, "/missing").found, false);
  assert.equal(jsonPointer(input, "/nested/empty/deeper").found, false);
  assert.deepEqual(jsonPointer({ value: null }, "/value"), {
    found: true,
    value: null,
  });
});

test("jsonEqual compares by JSON text, never across types", () => {
  assert.equal(jsonEqual(35, 35), true);
  assert.equal(jsonEqual(35, "35"), false);
  assert.equal(jsonEqual(true, "true"), false);
  assert.equal(jsonEqual(1.0, 1), true);
  assert.equal(jsonEqual({ a: 1, b: 2 }, { b: 2, a: 1 }), true);
  assert.equal(jsonEqual({ a: 1 }, { a: 1, b: 2 }), false);
  assert.equal(jsonEqual([1, 2], [2, 1]), false);
});

test("assertValue enforces each supported operator", () => {
  const input = {
    count: 0,
    error: { code: "INVALID_REQUEST" },
    items: ["a", "b"],
    name: "kitchen speaker",
    reading: "1700000000",
    volume: 35,
  };
  assertValue(input, {
    "/count": { non_empty: true },
    "/error/code": { equals: "INVALID_REQUEST" },
    "/items": { count: 2 },
    "/items/0": { contains: "a" },
    "/missing": { present: false },
    "/name": { max_length: 32, min_length: 3, pattern: "^kitchen" },
    "/reading": { minimum: 1_000_000_000 },
    "/volume": { equals: 35, maximum: 100, minimum: 0 },
  });
  assert.throws(
    () => assertValue(input, { "/volume": { equals: "35" } }),
    /equals failed/u,
  );
  assert.throws(
    () => assertValue(input, { "/items": { count: 3 } }),
    /count failed/u,
  );
  assert.throws(
    () => assertValue(input, { "/missing": { equals: 1 } }),
    /not found/u,
  );
  assert.throws(
    () => assertValue(input, { "/volume": { contains: "35" } }),
    /requires a string/u,
  );
  assert.throws(
    () => assertValue(input, { "/name": { minimum: 1 } }),
    /requires a numeric target/u,
  );
  // Matching the Go runner, only null, "", [] and {} count as empty, so a
  // zero number is non-empty.
  assert.throws(
    () => assertValue(input, { "/count": { non_empty: false } }),
    /non_empty failed/u,
  );
  assertValue({ list: [] }, { "/list": { non_empty: false } });
});

test("assertValue treats text-fragment arrays as joined text", () => {
  assertValue(
    { parts: ["hello ", "world"] },
    { "/parts": { contains: "lo wo" } },
  );
  assert.throws(
    () => assertValue({ parts: ["a", 1] }, { "/parts": { contains: "a" } }),
    /requires a string/u,
  );
});

test("generateValue matches the Go runner shapes", () => {
  const uuid = generateValue("uuid");
  assert.match(
    uuid,
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u,
  );
  for (const kind of ["token", "string"]) {
    const value = generateValue(kind);
    assert.equal(value.length, 33);
    assert.match(value, /^g[0-9a-f]{32}$/u);
  }
});

test("variables resolve inputs from value, env and generate", () => {
  const variables = new Variables(
    {
      endpoint: {
        direction: "input",
        env: "GIZTEST_UNIT_ENDPOINT",
        type: "string",
      },
      level: { direction: "input", type: "integer", value: 35 },
      name: { direction: "input", generate: "token", type: "string" },
    },
    { GIZTEST_UNIT_ENDPOINT: "127.0.0.1:9821" },
  );
  assert.equal(variables.resolve("${endpoint}"), "127.0.0.1:9821");
  assert.equal(variables.resolve("${level}"), 35);
  assert.match(String(variables.resolve("${name}")), /^g[0-9a-f]{32}$/u);
  assert.equal(
    variables.resolve("Bearer ${endpoint}/x"),
    "Bearer 127.0.0.1:9821/x",
  );
  assert.deepEqual(variables.resolve({ body: { level: "${level}" } }), {
    body: { level: 35 },
  });
  assert.deepEqual(variables.resolve(["${level}", 1]), [35, 1]);
});

test("variables reject a missing environment input", () => {
  assert.throws(
    () =>
      new Variables(
        {
          endpoint: {
            direction: "input",
            env: "GIZTEST_UNIT_ABSENT",
            type: "string",
          },
        },
        {},
      ),
    /requires environment/u,
  );
});

test("output variables assign once and gate references until captured", () => {
  const variables = new Variables({
    api_key: { direction: "output", secret: true, type: "string" },
  });
  assert.throws(() => variables.resolve("${api_key}"), /unavailable/u);
  variables.assign("api_key", "gizclaw_sk_v1_secret");
  assert.equal(variables.resolve("${api_key}"), "gizclaw_sk_v1_secret");
  assert.throws(
    () => variables.assign("api_key", "other"),
    /already assigned/u,
  );
  assert.throws(() => variables.assign("unknown", "x"), /unknown variable/u);
  assert.deepEqual(variables.redactions(), ["gizclaw_sk_v1_secret"]);
});

test("output variables reject a value of the wrong declared type", () => {
  const variables = new Variables({
    count: { direction: "output", type: "integer" },
  });
  assert.throws(() => variables.assign("count", "12"), /want integer/u);
  variables.assign("count", 12);
});

test("redactions sort longest first", () => {
  const variables = new Variables({
    long: { direction: "input", secret: true, type: "string", value: "abcdef" },
    short: { direction: "input", secret: true, type: "string", value: "abc" },
  });
  assert.deepEqual(variables.redactions(), ["abcdef", "abc"]);
});

test("safeError redacts secrets and blanks credential wording", () => {
  assert.equal(
    safeError(new Error("saw abcdef here"), ["abcdef"]),
    "saw [REDACTED] here",
  );
  assert.equal(
    safeError(new Error("bad Authorization header")),
    "redacted execution error",
  );
  assert.equal(safeError(new Error("x".repeat(600))).length, 512);
  assert.equal(safeError(undefined), "");
});

test("parseDuration accepts Go duration strings", () => {
  assert.equal(parseDuration("30s"), 30_000);
  assert.equal(parseDuration("2m30s"), 150_000);
  assert.equal(parseDuration("1.5h"), 5_400_000);
  assert.equal(parseDuration("250ms"), 250);
  assert.throws(() => parseDuration("soon"), /invalid duration/u);
});

test("httpBaseURL keeps the origin and drops path and query", () => {
  assert.equal(httpBaseURL("127.0.0.1:9821"), "http://127.0.0.1:9821");
  assert.equal(httpBaseURL("https://x.example/foo?a=1"), "https://x.example");
  assert.throws(() => httpBaseURL("  "), /invalid access point/u);
});

test("collectReferences finds every variable a step names", () => {
  const references = collectReferences({
    http: {
      body: { level: "${level}" },
      headers: { Authorization: "Bearer ${api_key}" },
      method: "PUT",
      path: "/gizclaw/v1/contacts/${resource_name}",
    },
    id: "step",
  });
  assert.deepEqual(references.sort(), ["api_key", "level", "resource_name"]);
});

test("loadDocument parses a real device control scenario", async () => {
  const document = await loadDocument(
    path.join(scenarioRoot, "server.device.volume.set.giztest.yaml"),
  );
  assert.equal(document.name, "server.device.volume.set");
  assert.equal(document.repeat, 1);
  assert.deepEqual(Object.keys(document.clients), ["peer"]);
  assert.deepEqual(
    document.steps.map((step) => stepOperation(step)),
    ["rpc", "rpc", "client_rpc", "http", "http", "rpc", "http"],
  );
  assert.equal(document.finally.length, 1);
  assert.equal(document.finally[0]?.id, "cleanup_peer");
});

test("loadDocuments loads every device, contact and API key scenario", async () => {
  const paths = await discover([scenarioRoot]);
  const selected = paths.filter((file) =>
    /server\.(device|contact|contacts|api_key)\./u.test(path.basename(file)),
  );
  assert.ok(selected.length >= 20, `selected ${selected.length} scenarios`);
  const { documents, skipped } = await loadDocuments(selected);
  assert.equal(skipped.length, 0, JSON.stringify(skipped));
  assert.equal(documents.length, selected.length);
});

test("loadDocuments skips scenarios that use unsupported step kinds", async () => {
  const paths = await discover([scenarioRoot]);
  const { skipped } = await loadDocuments(paths);
  assert.ok(
    skipped.length > 0,
    "expected speech or stream scenarios to be skipped",
  );
  for (const entry of skipped) {
    assert.match(entry.reason, /unsupported step kind/u);
  }
});

test("discover rejects a non-scenario file", async () => {
  await assert.rejects(
    discover([path.join(scenarioRoot, "..", "run_tests.sh")]),
    /not a \.giztest\.yaml file/u,
  );
});
