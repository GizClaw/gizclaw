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
