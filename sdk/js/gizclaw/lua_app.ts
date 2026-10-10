import type {
  ClientLuaAppInstallRequest,
  ClientLuaAppInstallStreamRequest,
  ClientLuaAppRunRequest,
  LuaAppInfo,
} from "./generated/rpc/payload-codec.ts";

const appID = /^[a-z0-9_-][a-z0-9_.-]{0,31}$/;
const version =
  /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
const bytes = (value: string): number => new TextEncoder().encode(value).length;
const text = (value: unknown, max: number): value is string =>
  typeof value === "string" && bytes(value) <= max && !value.includes("\0");
const object = (value: unknown): value is Record<string, unknown> =>
  value != null && typeof value === "object" && !Array.isArray(value);

export function validLuaAppInfo(value: unknown): value is LuaAppInfo {
  return (
    object(value) &&
    typeof value.app_id === "string" &&
    appID.test(value.app_id) &&
    text(value.version, 31) &&
    version.test(value.version) &&
    (value.display_name == null || text(value.display_name, 128)) &&
    (value.description == null || text(value.description, 1024))
  );
}

export function validLuaAppInstall(
  value: unknown,
): value is ClientLuaAppInstallRequest {
  if (!object(value) || !text(value.url, 256 * 1024)) return false;
  const source = value.url;
  const prefixes = [
    "data:application/zlib;base64,",
    "data:application/octet-stream;base64,",
  ];
  const prefix = prefixes.find((item) => source.startsWith(item));
  if (prefix != null) {
    const encoded = source.slice(prefix.length);
    if (
      !encoded ||
      !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(
        encoded,
      )
    )
      return false;
    const alphabet =
      "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    if (encoded.endsWith("==") && alphabet.indexOf(encoded.at(-3)!) & 15)
      return false;
    if (
      encoded.endsWith("=") &&
      !encoded.endsWith("==") &&
      alphabet.indexOf(encoded.at(-2)!) & 3
    )
      return false;
  } else {
    if (
      bytes(source) > 1024 ||
      /[\s\\#]/.test(source) ||
      !/^https?:\/\//.test(source)
    )
      return false;
    try {
      const url = new URL(source);
      if (
        (url.protocol !== "http:" && url.protocol !== "https:") ||
        !url.hostname ||
        url.username ||
        url.password ||
        source
          .slice(source.indexOf("://") + 3)
          .split(/[/?#]/)[0]
          ?.includes("@")
      )
        return false;
    } catch {
      return false;
    }
  }
  return (
    value.sha256 == null ||
    (typeof value.sha256 === "string" && /^[a-fA-F0-9]{64}$/.test(value.sha256))
  );
}

export function validLuaAppRun(
  value: unknown,
): value is ClientLuaAppRunRequest {
  if (
    !object(value) ||
    typeof value.app_id !== "string" ||
    !appID.test(value.app_id)
  )
    return false;
  if (value.params == null) return true;
  if (!object(value.params) || Object.keys(value.params).length > 16)
    return false;
  let total = 0;
  for (const [key, item] of Object.entries(value.params)) {
    if (key === "" || !text(key, 64) || !text(item, 1024)) return false;
    total += bytes(key) + bytes(item);
  }
  return total <= 4096;
}

export function validLuaAppUpload(
  value: unknown,
): value is ClientLuaAppInstallStreamRequest {
  return (
    object(value) &&
    typeof value.content_length === "number" &&
    Number.isInteger(value.content_length) &&
    value.content_length > 0 &&
    value.content_length <= 512 * 1024 &&
    typeof value.sha256 === "string" &&
    /^[a-fA-F0-9]{64}$/.test(value.sha256)
  );
}
