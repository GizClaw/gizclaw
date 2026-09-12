import assert from "node:assert/strict";
import { test } from "node:test";

import { aggregateLogs, compactLog } from "../src/logs.ts";
import type { LogEntry } from "../src/runtime.ts";

const at = (minute: number) =>
  Date.parse("2026-09-12T08:00:00Z") + minute * 60_000;

test("compactLog keeps the diagnostic fields and drops the rest", () => {
  const record = compactLog({
    time_ms: at(0),
    level: "ERROR",
    message: "request failed",
    fields: {
      method: "GET",
      request_path: "/gizclaw/v1/device/status",
      status: 403,
      rpc_code: "0",
      error_code: "DEBUG_ACCESS_FORBIDDEN",
      result: "success",
      duration_ms: "12",
      request_id: "req-1",
      secret_payload: "not for the model",
    },
  });
  assert.deepEqual(record, {
    time: "2026-09-12T08:00:00.000Z",
    level: "error",
    message: "request failed",
    operation: "GET /gizclaw/v1/device/status",
    http_status: "403",
    error_code: "DEBUG_ACCESS_FORBIDDEN",
    duration_ms: 12,
    request_id: "req-1",
  });
});

test("compactLog tolerates entries without fields", () => {
  assert.deepEqual(
    compactLog({ time_ms: at(1), level: "info", message: "hello" }),
    {
      time: "2026-09-12T08:01:00.000Z",
      level: "info",
      message: "hello",
    },
  );
  assert.equal(
    compactLog({
      time_ms: at(1),
      level: "info",
      message: "x",
      fields: { operation: "unknown", route: "/a" },
    }).operation,
    "/a",
  );
});

test("aggregateLogs counts by every dimension and ranks errors", () => {
  const entries: LogEntry[] = [
    {
      time_ms: at(1),
      level: "error",
      message: "old timeout",
      fields: { operation: "asr", error_code: "ASR_TIMEOUT" },
    },
    {
      time_ms: at(5),
      level: "error",
      message: "new timeout",
      fields: { operation: "asr", error_code: "ASR_TIMEOUT" },
    },
    {
      time_ms: at(3),
      level: "error",
      message: "bad gateway",
      fields: { operation: "tts", error_code: "TTS", status: 502 },
    },
    {
      time_ms: at(2),
      level: "warn",
      message: "slow",
      fields: { rpc_code: "4" },
    },
    { time_ms: at(4), level: "info", message: "ok" },
  ];
  const aggregate = aggregateLogs(entries);
  assert.equal(aggregate.total, 5);
  assert.equal(aggregate.first_time, "2026-09-12T08:01:00.000Z");
  assert.equal(aggregate.last_time, "2026-09-12T08:05:00.000Z");
  assert.deepEqual(aggregate.by_level, { error: 3, warn: 1, info: 1 });
  assert.deepEqual(aggregate.by_operation, { asr: 2, tts: 1 });
  assert.deepEqual(aggregate.by_error_code, { ASR_TIMEOUT: 2, TTS: 1 });
  assert.deepEqual(aggregate.by_rpc_code, { "4": 1 });
  assert.deepEqual(aggregate.by_http_status, { "502": 1 });
  assert.deepEqual(aggregate.top_errors, [
    { error_code: "ASR_TIMEOUT", count: 2, last_message: "new timeout" },
    { error_code: "TTS", count: 1, last_message: "bad gateway" },
  ]);
});

test("aggregateLogs of nothing has no time range", () => {
  const aggregate = aggregateLogs([]);
  assert.equal(aggregate.total, 0);
  assert.equal(aggregate.first_time, undefined);
  assert.deepEqual(aggregate.top_errors, []);
});
