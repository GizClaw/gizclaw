import assert from "node:assert/strict";
import test from "node:test";
import {
  type PeerStatus,
  encodeRPCResponsePayload,
  decodeRPCResponsePayload,
} from "./generated/rpc/payload-codec.ts";
import { encodeTelemetryPacket, otaTelemetry, OtaState } from "./telemetry.ts";

test("OTA packet matches protoc including zero progress and negative time delta", () => {
  const observation = otaTelemetry({
    state: OtaState.OTA_STATE_FAILED,
    updateId: "ota-1",
    targetVersion: "2.0",
    downloadPercent: 0,
    errorCode: "E_DOWNLOAD",
    errorMessage: "timeout",
  });
  observation.observedAtDeltaMs = -2;
  const packet = encodeTelemetryPacket({
    sequence: 7,
    observedAtUnixMs: 1234,
    observations: [observation],
  });
  assert.equal(
    Buffer.from(packet).toString("hex"),
    "40080710d2091a3908feffffffffffffffff01722c080412056f74612d311a03322e302100000000000000002a0a455f444f574e4c4f4144320774696d656f7574",
  );
});

test("all OTA states can be sent, including absent progress", () => {
  for (const state of [
    OtaState.OTA_STATE_STARTED,
    OtaState.OTA_STATE_DOWNLOADING,
    OtaState.OTA_STATE_SUCCEEDED,
    OtaState.OTA_STATE_FAILED,
  ]) {
    assert.equal(
      encodeTelemetryPacket({
        observations: [
          otaTelemetry({
            state,
            updateId: "ota-1",
            ...(state === OtaState.OTA_STATE_DOWNLOADING
              ? { downloadPercent: 100 }
              : {}),
          }),
        ],
      })[0],
      0x40,
    );
  }
});

test("RPC OTA status preserves zero progress and future state values", () => {
  for (const state of ["downloading", "future-state"]) {
    const response = {
      labels: {},
      ota: {
        state,
        update_id: "one",
        observed_at: "2026-09-06T00:00:00Z",
        download_percent: 0,
        target_version: "2.0",
      },
    };
    assert.deepEqual(
      decodeRPCResponsePayload(
        "server.status.get",
        encodeRPCResponsePayload("server.status.get", response),
      ),
      response,
    );
  }
});

test("RPC status without an OTA snapshot omits ota", () => {
  const otaFields: Pick<PeerStatus, "ota"> = {};
  const response = { labels: {}, ...otaFields };
  assert.deepEqual(
    decodeRPCResponsePayload(
      "server.status.get",
      encodeRPCResponsePayload("server.status.get", response),
    ),
    response,
  );
});

test("RPC status round-trips cellular identity strings", () => {
  const response = {
    labels: {},
    network_imei: "490154203237518",
    network_imsi: "460001234567890",
  };
  assert.deepEqual(
    decodeRPCResponsePayload(
      "server.status.get",
      encodeRPCResponsePayload("server.status.get", response),
    ),
    response,
  );
});

test("KV telemetry preserves JSON and enforces bounded values and frames", async () => {
  const { kvTelemetry, kvTelemetryJSON, deleteKVTelemetry } =
    await import("./telemetry.ts");
  const hex = (bytes: Uint8Array) => Buffer.from(bytes).toString("hex");
  assert.equal(
    hex(
      encodeTelemetryPacket({
        sequence: 7,
        observedAtUnixMs: 1234,
        observations: [kvTelemetry("k", null)],
      }),
    ),
    "40080710d2091a0c8a01090a016b12046e756c6c",
  );
  for (const value of [
    null,
    false,
    0,
    { x: [null, false, 0] },
    [1, 2],
    "text",
  ]) {
    const observation = kvTelemetry("state", value);
    assert.deepEqual(JSON.parse(observation.kv!.jsonValue!), value);
    assert.doesNotThrow(() =>
      encodeTelemetryPacket({ observations: [observation] }),
    );
  }
  assert.equal(kvTelemetryJSON("numeric", "1e400").kv?.jsonValue, "1e400");
  assert.equal(deleteKVTelemetry("k").kv?.delete, true);
  assert.equal(deleteKVTelemetry("k").kv?.jsonValue, undefined);
  assert.throws(() => kvTelemetry("中".repeat(22), 0));
  assert.doesNotThrow(() => kvTelemetry("x".repeat(64), "x".repeat(1022)));
  assert.throws(() => kvTelemetry("k", "x".repeat(1023)));
  assert.throws(() => kvTelemetryJSON("k", "{bad}"));
  assert.throws(() =>
    encodeTelemetryPacket({
      observations: Array.from({ length: 17 }, () => kvTelemetry("k", 0)),
    }),
  );
  assert.throws(() =>
    encodeTelemetryPacket({
      observations: Array.from({ length: 4 }, () =>
        kvTelemetry("k", "x".repeat(1022)),
      ),
    }),
  );
});

test("KV convenience encoding rejects non-JSON values without dropping fields", async () => {
  const { kvTelemetry } = await import("./telemetry.ts");
  for (const value of [
    NaN,
    Infinity,
    undefined,
    { x: undefined },
    [undefined],
    { x: () => 0 },
  ])
    assert.throws(() => kvTelemetry("k", value));
});
