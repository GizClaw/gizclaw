import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";

import { assertValue, jsonEqual, jsonPointer, safeError } from "./assert.ts";
import {
  MAX_SCRIPTED_DELAY_MS,
  httpBaseURL,
  scriptedDelayMs,
} from "./client.ts";
import {
  collectReferences,
  discover,
  loadDocument,
  loadDocuments,
  parseDuration,
  stepOperation,
} from "./document.ts";
import { telemetryFrameFromProtoJSON } from "./proto_json.ts";
import { AssertionFailure } from "./assert.ts";
import { failureKind } from "./runner.ts";
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
  // A token must parse whole, so a numeric prefix does not index the array.
  assert.equal(jsonPointer(input, "/items/1junk").found, false);
  assert.equal(jsonPointer(input, "/items/ 1").found, false);
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
  // The client_rpc step asserts the call count, so it follows the HTTP step
  // that makes the server call the device.
  assert.deepEqual(
    document.steps.map((step) => stepOperation(step)),
    ["rpc", "rpc", "http", "http", "rpc", "http", "client_rpc"],
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
  assert.deepEqual(skipped, []);
  assert.equal(documents.length, selected.length);
});

test("loadDocuments loads the telemetry scenarios", async () => {
  const names = [
    "server.device.audioplayer.telemetry",
    "server.device.telemetry.status",
  ];
  const { documents, skipped } = await loadDocuments(
    names.map((name) => path.join(scenarioRoot, `${name}.giztest.yaml`)),
  );
  assert.deepEqual(skipped, []);
  assert.deepEqual(
    documents.map((document) => document.name),
    names,
  );
  // Every frame the scenarios send must convert for the SDK.
  for (const document of documents) {
    for (const step of document.steps) {
      if (step.telemetry != null) {
        const frame = telemetryFrameFromProtoJSON(step.telemetry.frame);
        assert.ok((frame.observations?.length ?? 0) > 0, step.id);
      }
    }
  }
});

test("loadDocuments loads the find, social ping and profile scenarios", async () => {
  const names = [
    "server.device.find",
    "server.device.find.unsupported",
    "server.friend.ping",
    "server.friend_group.ping",
    "server.profile.get",
  ];
  const { documents, skipped } = await loadDocuments(
    names.map((name) => path.join(scenarioRoot, `${name}.giztest.yaml`)),
  );
  assert.deepEqual(skipped, []);
  assert.deepEqual(
    documents.map((document) => document.name),
    names,
  );
  const providers = documents.flatMap((document) =>
    document.steps.flatMap((step) =>
      step.client_rpc == null ? [] : [step.client_rpc.method],
    ),
  );
  assert.deepEqual(providers, [
    "client.device.find",
    "client.device.find",
    "client.social.ping",
    "client.social.ping",
  ]);
});

test("loadDocuments loads the device settings, reset, methods, workspace and tool scenarios", async () => {
  // loadDocuments orders documents by path.
  const names = [
    "server.device.factory_reset",
    "server.device.rpc_methods",
    "server.device.run_workspace.set",
    "server.device.settings",
    "server.device.tools",
  ];
  const { documents, skipped } = await loadDocuments(
    names.map((name) => path.join(scenarioRoot, `${name}.giztest.yaml`)),
  );
  assert.deepEqual(skipped, []);
  assert.deepEqual(
    documents.map((document) => document.name),
    names,
  );
  const providers = documents.flatMap((document) =>
    document.steps.flatMap((step) =>
      step.client_rpc == null ? [] : [step.client_rpc.method],
    ),
  );
  assert.deepEqual(providers, [
    "client.device.factory_reset",
    "client.device.settings.get",
    "client.device.settings.get",
    "client.device.find",
    "client.run.workspace.set",
    "client.device.settings.get",
    "client.device.settings.set",
    "client.tool.invoke",
  ]);
  const methods = new Set(
    documents.flatMap((document) =>
      document.steps.flatMap((step) =>
        step.http == null ? [] : [step.http.method],
      ),
    ),
  );
  assert.ok(methods.has("PATCH"), [...methods].join(","));
});

// loadRetryDocument loads a one-step rpc document whose step carries retry, or a
// finalizer carrying it when inFinally is set.
async function loadRetryDocument(
  retry: string,
  options: { operation?: string; inFinally?: boolean } = {},
): Promise<unknown> {
  const directory = await mkdtemp(path.join(tmpdir(), "giztest-"));
  try {
    const file = path.join(directory, "retry.giztest.yaml");
    const operation =
      options.operation ?? "rpc: {method: server.status.get, request: {}}";
    const step = [
      "- id: poll",
      "  client: peer",
      `  ${operation}`,
      `  retry: ${retry}`,
    ];
    await writeFile(
      file,
      [
        "# User Story:",
        "# As a Giztest author,",
        "# I want retry policies validated like the Go runner,",
        "# So that a telemetry scenario can poll status the same way everywhere.",
        "version: gizclaw.test/v1alpha1",
        "name: retry",
        "clients:",
        "  peer: {identity: ephemeral, connection: webrtc, access_point: 127.0.0.1:1}",
        "steps:",
        ...(options.inFinally
          ? [
              "- id: first",
              "  client: peer",
              "  rpc: {method: server.status.get, request: {}}",
            ]
          : step),
        ...(options.inFinally ? ["finally:", ...step] : []),
        "",
      ].join("\n"),
    );
    return await loadDocument(file);
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
}

test("loadDocument accepts a retry policy like the Go runner", async () => {
  const document = (await loadRetryDocument(
    "{attempts: 10, on: [assertion], delay: 200ms}",
  )) as { steps: { retry?: unknown }[] };
  assert.deepEqual(document.steps[0]?.retry, {
    attempts: 10,
    delay: "200ms",
    on: ["assertion"],
  });
  await loadRetryDocument("{attempts: 2}");
});

test("loadDocument rejects retry policies the Go runner rejects", async () => {
  for (const [retry, pattern] of [
    ["{attempts: 1}", /attempts must be between/u],
    ["{attempts: 11}", /attempts must be between/u],
    ["{attempts: 2, on: []}", /must not be empty/u],
    ["{attempts: 2, on: [operation]}", /unsupported failure kind/u],
    ["{attempts: 2, on: [timeout, timeout]}", /must be unique/u],
    ["{attempts: 2, delay: 0ms}", /invalid delay/u],
    ["{attempts: 2, delay: 6m}", /invalid delay/u],
  ] as const) {
    await assert.rejects(loadRetryDocument(retry), pattern, retry);
  }
  await assert.rejects(
    loadRetryDocument("{attempts: 2}", {
      operation: "reconnect: {}",
    }),
    /does not support retry/u,
  );
  await assert.rejects(
    loadRetryDocument("{attempts: 2}", { inFinally: true }),
    /not allowed in finally/u,
  );
});

test("failureKind classifies failures for retry like the Go runner", () => {
  const live = new AbortController().signal;
  assert.equal(failureKind(undefined, live), "");
  assert.equal(failureKind(new AssertionFailure("x"), live), "assertion");
  const timeout = new Error("timed out");
  timeout.name = "TimeoutError";
  assert.equal(failureKind(timeout, live), "timeout");
  assert.equal(
    failureKind(new Error("wrapped", { cause: timeout }), live),
    "timeout",
  );
  assert.equal(failureKind(new Error("boom"), live), "operation");
  const aborted = new AbortController();
  aborted.abort();
  assert.equal(failureKind(new Error("boom"), aborted.signal), "cancelled");
});

test("Variables restore discards a failed attempt's captures", () => {
  const variables = new Variables({
    captured: { direction: "output", type: "string" },
  });
  const snapshot = variables.snapshot();
  variables.assign("captured", "first");
  variables.restore(snapshot);
  assert.equal(variables.get("captured")?.data, undefined);
  variables.assign("captured", "second");
  assert.equal(variables.get("captured")?.data, "second");
});

test("loadDocument skips a scripted client.rpc.methods.get step", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "giztest-"));
  try {
    const file = path.join(directory, "methods.giztest.yaml");
    await writeFile(
      file,
      [
        "# User Story:",
        "# As a Giztest author,",
        "# I want a scripted client.rpc.methods.get step reported,",
        "# So that this runner does not wait on calls it cannot count.",
        "version: gizclaw.test/v1alpha1",
        "name: methods",
        "clients:",
        "  peer: {identity: ephemeral, connection: webrtc, access_point: 127.0.0.1:1}",
        "steps:",
        "- id: methods",
        "  client: peer",
        "  client_rpc: {method: client.rpc.methods.get}",
        "",
      ].join("\n"),
    );
    const { documents, skipped } = await loadDocuments([file]);
    assert.deepEqual(documents, []);
    assert.match(skipped[0]!.reason, /client\.rpc\.methods\.get/u);
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("loadDocuments loads the friend and friend group HTTP scenarios", async () => {
  const names = ["server.friend_groups.http", "server.friends.http"];
  const { documents, skipped } = await loadDocuments(
    names.map((name) => path.join(scenarioRoot, `${name}.giztest.yaml`)),
  );
  assert.deepEqual(skipped, []);
  assert.deepEqual(
    documents.map((document) => document.name),
    names,
  );
  const operations = new Set(
    documents.flatMap((document) =>
      document.steps.map((step) => stepOperation(step)),
    ),
  );
  assert.deepEqual([...operations].sort(), ["http", "rpc"]);
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

// A scripted delay must never silently become an immediate answer: a scenario
// that scripts one is written to exercise a timeout, and Node clamps an
// out-of-range setTimeout delay to a single millisecond.
test("scriptedDelayMs bounds the delay to the timer range", () => {
  assert.equal(scriptedDelayMs({}), 0);
  assert.equal(scriptedDelayMs({ delay_ms: 0 }), 0);
  assert.equal(scriptedDelayMs({ delay_ms: 1200 }), 1200);
  assert.equal(
    scriptedDelayMs({ delay_ms: MAX_SCRIPTED_DELAY_MS }),
    MAX_SCRIPTED_DELAY_MS,
  );
  assert.throws(() => scriptedDelayMs({ delay_ms: MAX_SCRIPTED_DELAY_MS + 1 }));
  assert.throws(() => scriptedDelayMs({ delay_ms: Number.MAX_SAFE_INTEGER }));
  assert.throws(() => scriptedDelayMs({ delay_ms: -1 }));
  assert.throws(() => scriptedDelayMs({ delay_ms: 1.5 }));
  assert.throws(() => scriptedDelayMs({ delay_ms: "1200" }));
});

test("MHS v0 scenarios install typed providers and preserve real error coverage", async () => {
  const paths = [
    "server.device.mhs.giztest.yaml",
    "server.device.mhs.not_found.giztest.yaml",
  ].map((name) => path.join(scenarioRoot, name));
  const { documents, skipped } = await loadDocuments(paths);
  assert.equal(skipped.length, 0);
  assert.equal(documents.length, 2);
});
