import assert from "node:assert/strict";
import test from "node:test";
import { equals, fromBinary, fromJsonString } from "@bufbuild/protobuf";
import { encodeTelemetryPacket } from "@gizclaw/gizclaw";
import { encodeRPCRequestPayload } from "../../../../sdk/js/gizclaw/generated/rpc/payload-codec.ts";
import {
  requestFromProtoJSON,
  responseToProtoJSON,
  telemetryFrameFromProtoJSON,
  telemetryFrameSchema,
} from "./proto_json.ts";

test("scenario Protobuf JSON converts workspace oneofs and enums for the SDK", () => {
  const request = requestFromProtoJSON("server.workspace.create", {
    name: "example",
    workflow_name: "flowcraft-chat-assistant",
    parameters: {
      flowcraft_workspace_parameters: {
        agent_type: "FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_FLOWCRAFT",
        input: "WORKSPACE_INPUT_MODE_PUSH_TO_TALK",
      },
    },
  });
  assert.ok(
    request != null && typeof request === "object" && "parameters" in request,
  );
  assert.ok(
    request.parameters != null && typeof request.parameters === "object",
  );
  assert.ok(
    "agent_type" in request.parameters && "input" in request.parameters,
  );
  assert.equal(request.parameters.agent_type, "flowcraft");
  assert.equal(request.parameters.input, "push-to-talk");
});

test("scenario enum conversion retains strict Protobuf JSON validation", () => {
  assert.deepEqual(
    requestFromProtoJSON("server.firmware.get", {
      channel: "FIRMWARE_CHANNEL_NAME_STABLE",
    }),
    { channel: "stable" },
  );
  assert.throws(() =>
    requestFromProtoJSON("server.firmware.get", {
      channel: "FIRMWARE_CHANNEL_NAME_STABEL",
    }),
  );
  assert.throws(() =>
    requestFromProtoJSON("server.firmware.get", {
      unknown_field: true,
    }),
  );
});

test("invalid numeric safety fence values survive the SDK request encoder", () => {
  const method = "server.workspace.parameters.set";
  const request = requestFromProtoJSON(method, {
    name: "x",
    parameters: { safety_fence_level: 99 },
  });
  // WorkspaceParametersSetRequest.parameters is field 2; its optional
  // safety_fence_level is field 4. The Server must receive 99, not an unset
  // field or a locally rejected empty string.
  assert.deepEqual(
    encodeRPCRequestPayload(method, request),
    new Uint8Array([0x0a, 0x01, 0x78, 0x12, 0x02, 0x20, 0x63]),
  );
  for (const value of ["", "SAFETY_FENCE_LEVEL_UNKNOWN"]) {
    assert.throws(() =>
      requestFromProtoJSON(method, {
        parameters: { safety_fence_level: value },
      }),
    );
  }
  assert.deepEqual(
    encodeRPCRequestPayload(
      method,
      requestFromProtoJSON(method, { name: "x" }),
    ),
    new Uint8Array([0x0a, 0x01, 0x78]),
  );
});

test("SDK response conversion retains protobuf int64 strings and defaults", () => {
  const response = responseToProtoJSON("all.ping", { server_time: 123 });
  assert.ok(
    response != null &&
      typeof response === "object" &&
      "server_time" in response,
  );
  assert.equal(response.server_time, "123");
});

test("social ping responses keep Protobuf JSON enum names and optional presence", () => {
  assert.deepEqual(
    responseToProtoJSON("server.friend.ping", {
      result: "delivered",
      delivered_count: 1,
    }),
    { result: "SOCIAL_PING_RESULT_DELIVERED", delivered_count: 1 },
  );
  assert.deepEqual(
    responseToProtoJSON("server.friend_group.ping", {
      result: "rate_limited",
      delivered_count: 0,
      retry_after_seconds: 42,
    }),
    {
      result: "SOCIAL_PING_RESULT_RATE_LIMITED",
      delivered_count: 0,
      retry_after_seconds: 42,
    },
  );
});

test("profile lookups convert keys and public profiles", () => {
  assert.deepEqual(
    requestFromProtoJSON("server.profile.get", {
      peer_public_keys: ["carol-key"],
    }),
    { peer_public_keys: ["carol-key"] },
  );
  assert.deepEqual(
    responseToProtoJSON("server.profile.get", {
      items: [
        { peer_public_key: "carol-key", display_name: "Carol", emoji: "🐱" },
        { peer_public_key: "dave-key" },
      ],
    }),
    {
      items: [
        { peer_public_key: "carol-key", display_name: "Carol", emoji: "🐱" },
        { peer_public_key: "dave-key" },
      ],
    },
  );
});

test("telemetry Protobuf JSON survives the SDK's telemetry encoder", () => {
  const input = {
    sequence: 7,
    observed_at_unix_ms: "1800000000000",
    observations: [
      { activity: { activity: "chat", detail: "bedtime story" } },
      { system: { firmware_version: "" } },
      { network: { rat: "lte", rssi_dbm: -90, signal_level: 3 } },
      { battery: { percent: 61 }, observed_at_delta_ms: -2000 },
      { gnss: { latitude: 1.5, longitude: -2.5, accuracy_m: 4 } },
      {
        audioplayer: {
          state: "error",
          repeat: "all",
          playlist_length: 1,
          playlist_revision: 1,
          current_index: 0,
          position_ms: "12000",
          error_code: "FETCH_FAILED",
        },
      },
      {
        ota: {
          state: "OTA_STATE_DOWNLOADING",
          update_id: "attempt-1",
          download_percent: 50,
        },
      },
    ],
  };
  const frame = telemetryFrameFromProtoJSON(input);
  assert.equal(frame.observedAtUnixMs, 1800000000000);
  assert.deepEqual(frame.observations?.[1], {
    observedAtDeltaMs: 0,
    system: { firmwareVersion: "" },
  });
  assert.equal(frame.observations?.[6]?.ota?.state, 2);
  const schema = telemetryFrameSchema();
  const packet = encodeTelemetryPacket(frame);
  assert.ok(
    equals(
      schema,
      fromBinary(schema, packet.slice(1)),
      fromJsonString(schema, JSON.stringify(input)),
    ),
  );
});

test("telemetry Protobuf JSON rejects empty frames and unknown members", () => {
  assert.throws(
    () => telemetryFrameFromProtoJSON({ observations: [] }),
    /requires observations/u,
  );
  assert.throws(
    () => telemetryFrameFromProtoJSON({ observations: [{}] }),
    /requires observation bodies/u,
  );
  assert.throws(() =>
    telemetryFrameFromProtoJSON({
      observations: [{ battery: { precent: 1 } }],
    }),
  );
});
