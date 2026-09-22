// Giztest documents use Protobuf JSON, while the JS SDK exposes domain-shaped
// objects. Convert through the authoritative wire schema in both directions.
import { readFileSync } from "node:fs";
import {
  createFileRegistry,
  fromBinary,
  fromJsonString,
  getOption,
  hasOption,
  isMessage,
  toBinary,
  toJson,
  type DescField,
  type DescMessage,
} from "@bufbuild/protobuf";
import { FileDescriptorSetSchema } from "@bufbuild/protobuf/wkt";
import type { TelemetryFrame } from "@gizclaw/gizclaw";
import {
  decodeClientToolRequestPayload,
  encodeClientToolResponsePayload,
  decodeRPCRequestPayload,
  encodeRPCResponsePayload,
} from "../../../../sdk/js/gizclaw/generated/rpc/payload-codec.ts";

const registry = createFileRegistry(
  fromBinary(
    FileDescriptorSetSchema,
    readFileSync(new URL("../../testdata/rpc-descriptors.pb", import.meta.url)),
  ),
);
const methods = new Map<
  string,
  { request: DescMessage; response: DescMessage; id: number }
>();
const tools = new Map<string, { request: DescMessage; response: DescMessage; id: number }>();
loadRegistry("RpcMethod", "rpc_method", methods);
loadRegistry("ClientTool", "client_tool", tools);
function loadRegistry(enumName: string, optionName: string, index: Map<string, { request: DescMessage; response: DescMessage; id: number }>): void {
const methodEnum = registry.getEnum(`gizclaw.rpc.v1.${enumName}`);
const methodOption = registry.getExtension(`gizclaw.rpc.v1.${optionName}`);
if (methodEnum == null || methodOption?.fieldKind !== "message") {
  throw new Error("RPC descriptor set is missing method metadata");
}
for (const value of methodEnum.values) {
  if (!hasOption(value, methodOption)) continue;
  const option = getOption(value, methodOption);
  if (!isMessage(option)) throw new Error("invalid RPC method option");
  const meta = toJson(methodOption.message, option);
  if (
    meta == null ||
    typeof meta !== "object" ||
    Array.isArray(meta) ||
    typeof meta.name !== "string" ||
    typeof meta.request !== "string" ||
    typeof meta.response !== "string"
  ) {
    throw new Error("invalid RPC method metadata");
  }
  const request = registry.getMessage(`gizclaw.rpc.v1.${meta.request}`);
  const response = registry.getMessage(`gizclaw.rpc.v1.${meta.response}`);
  if (request == null || response == null)
    throw new Error(`missing payload for ${meta.name}`);
  index.set(meta.name, { request, response, id: value.number });
}

}

function payloadSchema(
  method: string,
  direction: "request" | "response",
): DescMessage {
  const schema = methods.get(method)?.[direction];
  if (schema == null) throw new Error(`unknown RPC method: ${method}`);
  return schema;
}

function isValueWrapper(schema: DescMessage): boolean {
  return (
    schema.fields.length === 1 &&
    schema.fields[0]?.name === "value" &&
    schema.fields[0].fieldKind === "message"
  );
}

export function requestFromProtoJSON(method: string, input: unknown): unknown {
  const schema = payloadSchema(method, "request");
  const wrapped =
    input != null && typeof input === "object" && Object.hasOwn(input, "value");
  const value = isValueWrapper(schema) && !wrapped ? { value: input } : input;
  const message = fromJsonString(schema, JSON.stringify(value ?? {}));
  return decodeRPCRequestPayload(method, toBinary(schema, message));
}

export function responseToProtoJSON(method: string, input: unknown): unknown {
  const schema = payloadSchema(method, "response");
  const message = fromBinary(schema, encodeRPCResponsePayload(method, input));
  const value = toJson(schema, message, {
    useProtoFieldName: true,
    alwaysEmitImplicit: true,
  });
  if (
    isValueWrapper(schema) &&
    value != null &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    Object.hasOwn(value, "value")
  )
    return value.value;
  return value;
}

const telemetryRegistry = createFileRegistry(
  fromBinary(
    FileDescriptorSetSchema,
    readFileSync(
      new URL("../../testdata/telemetry-descriptors.pb", import.meta.url),
    ),
  ),
);

export function telemetryFrameSchema(): DescMessage {
  const schema = telemetryRegistry.getMessage(
    "gizclaw.telemetry.v1.TelemetryFrame",
  );
  if (schema == null) throw new Error("telemetry descriptor set is missing");
  return schema;
}

// telemetryFrameFromProtoJSON reads a telemetry step's Protobuf JSON frame
// with the same strict parsing as the Go runner and reshapes it into the JS
// SDK's TelemetryFrame: camelCase members, the oneof body as its own member,
// 64-bit integers as numbers and enums as their numbers.
export function telemetryFrameFromProtoJSON(input: unknown): TelemetryFrame {
  const schema = telemetryFrameSchema();
  const message = fromJsonString(schema, JSON.stringify(input ?? {}));
  const frame = sdkMessage(schema, message) as TelemetryFrame;
  const observations = frame.observations ?? [];
  if (observations.length === 0) {
    throw new Error("telemetry requires observations");
  }
  for (const observation of observations) {
    if (Object.keys(observation).every((key) => key === "observedAtDeltaMs")) {
      throw new Error("telemetry requires observation bodies");
    }
  }
  return frame;
}

function sdkMessage(
  schema: DescMessage,
  message: unknown,
): Record<string, unknown> {
  const source = message as Record<string, unknown>;
  const out: Record<string, unknown> = {};
  for (const member of schema.members) {
    if (member.kind === "oneof") {
      const selected = source[member.localName] as
        { case?: string; value?: unknown } | undefined;
      const field = member.fields.find(
        (candidate) => candidate.localName === selected?.case,
      );
      if (field != null) {
        out[field.localName] = sdkValue(field, selected?.value);
      }
      continue;
    }
    const value = source[member.localName];
    if (value !== undefined) {
      out[member.localName] = sdkValue(member, value);
    }
  }
  return out;
}

function sdkValue(field: DescField, value: unknown): unknown {
  switch (field.fieldKind) {
    case "message":
      return sdkMessage(field.message, value);
    case "list":
      return (value as unknown[]).map((item) =>
        field.listKind === "message"
          ? sdkMessage(field.message, item)
          : sdkScalar(item),
      );
    case "map":
      throw new Error(`telemetry map field ${field.name} is unsupported`);
    default:
      return sdkScalar(value);
  }
}

function sdkScalar(value: unknown): unknown {
  return typeof value === "bigint" ? Number(value) : value;
}

export function toolRequestFromProtoJSON(tool: string, input: unknown): unknown {
 const info = tools.get(tool); if (info == null) throw new Error(`unknown tool ${tool}`);
 const message = fromJsonString(info.request, JSON.stringify(input ?? {}));
 return decodeClientToolRequestPayload(info.id, toBinary(info.request, message));
}
export function toolResponseToProtoJSON(tool: string, input: unknown): unknown {
 const info = tools.get(tool); if (info == null) throw new Error(`unknown tool ${tool}`);
 return toJson(info.response, fromBinary(info.response, encodeClientToolResponsePayload(info.id, input)), {useProtoFieldName:true, alwaysEmitImplicit:true});
}
