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
  type DescMessage,
} from "@bufbuild/protobuf";
import { FileDescriptorSetSchema } from "@bufbuild/protobuf/wkt";
import {
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
  { request: DescMessage; response: DescMessage }
>();
const methodEnum = registry.getEnum("gizclaw.rpc.v1.RpcMethod");
const methodOption = registry.getExtension("gizclaw.rpc.v1.rpc_method");
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
  methods.set(meta.name, { request, response });
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
