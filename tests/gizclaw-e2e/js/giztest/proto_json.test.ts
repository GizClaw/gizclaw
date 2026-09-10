import assert from "node:assert/strict";
import test from "node:test";
import { requestFromProtoJSON, responseToProtoJSON } from "./proto_json.ts";

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
