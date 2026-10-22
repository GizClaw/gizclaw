// Shared validation for predefined device management invoke providers.
import {
  CLIENT_TOOL_IDS,
  type ClientToolID,
} from "./generated/rpc/method-map.ts";

export const isManagementTool = (tool: number): boolean =>
  tool >= 28 && tool <= 38;
const object = (value: unknown): Record<string, unknown> => {
  if (value == null || typeof value !== "object" || Array.isArray(value))
    throw new Error("invalid management result");
  return value as Record<string, unknown>;
};
const text = (value: unknown, max: number): value is string =>
  typeof value === "string" &&
  value.length > 0 &&
  value.isWellFormed() &&
  !value.includes("\0") &&
  new TextEncoder().encode(value).length <= max;
const uint32 = (value: unknown): value is number =>
  typeof value === "number" &&
  Number.isInteger(value) &&
  value >= 0 &&
  value <= 0xffffffff;
function requireValue(valid: boolean): void {
  if (!valid) throw new Error("invalid management request or result");
}

export function validateManagementRequest(
  tool: ClientToolID,
  value: unknown,
): void {
  const request = object(value);
  switch (tool) {
    case CLIENT_TOOL_IDS["content.age.set"]:
      requireValue(text(request.option_id, 64));
      break;
    case CLIENT_TOOL_IDS["device.feature.set"]:
      requireValue(
        text(request.feature_id, 64) && typeof request.enabled === "boolean",
      );
      break;
    case CLIENT_TOOL_IDS["usage.time.set"]:
      requireValue(uint32(request.limit_seconds));
      break;
    case CLIENT_TOOL_IDS["ai.safety.set"]:
      requireValue(text(request.level, 64));
      break;
    case CLIENT_TOOL_IDS["management.password.set"]:
      requireValue(text(request.password, 128));
      break;
  }
}
function feature(value: unknown): Record<string, unknown> {
  const item = object(value);
  requireValue(
    text(item.id, 64) &&
      text(item.display_name, 128) &&
      typeof item.enabled === "boolean" &&
      typeof item.writable === "boolean",
  );
  return item;
}
function options(value: unknown, age: boolean): void {
  const state = object(value);
  const selected = age ? state.option_id : state.level;
  requireValue(
    text(selected, 64) &&
      Array.isArray(state.options) &&
      state.options.length > 0 &&
      state.options.length <= 16,
  );
  const entries = state.options as unknown[];
  const seen = new Set<string>();
  for (const value of entries) {
    const item = object(value);
    requireValue(
      text(item.id, 64) &&
        text(item.display_name, 128) &&
        !seen.has(item.id as string),
    );
    seen.add(item.id as string);
    if (age)
      requireValue(
        (item.min_age_years === undefined || uint32(item.min_age_years)) &&
          (item.max_age_years === undefined || uint32(item.max_age_years)) &&
          (item.min_age_years === undefined ||
            item.max_age_years === undefined ||
            Number(item.min_age_years) <= Number(item.max_age_years)),
      );
  }
  requireValue(seen.has(selected as string));
}
export function validateManagementResult(
  tool: ClientToolID,
  input: unknown,
  value: unknown,
): void {
  const result = object(value),
    request = object(input);
  switch (tool) {
    case CLIENT_TOOL_IDS["content.age.get"]:
      options(result.state, true);
      break;
    case CLIENT_TOOL_IDS["content.age.set"]:
      options(result.applied, true);
      break;
    case CLIENT_TOOL_IDS["ai.safety.get"]:
      options(result.state, false);
      break;
    case CLIENT_TOOL_IDS["ai.safety.set"]:
      options(result.applied, false);
      break;
    case CLIENT_TOOL_IDS["device.features.get"]: {
      requireValue(
        Array.isArray(result.features) && result.features.length <= 32,
      );
      const seen = new Set();
      for (const item of result.features as unknown[]) {
        const f = feature(item);
        requireValue(!seen.has(f.id));
        seen.add(f.id);
      }
      break;
    }
    case CLIENT_TOOL_IDS["device.feature.set"]: {
      const item = feature(result.applied);
      requireValue(item.id === request.feature_id && item.writable === true);
      break;
    }
    case CLIENT_TOOL_IDS["usage.time.get"]:
    case CLIENT_TOOL_IDS["usage.time.set"]: {
      const state = object(
        tool === CLIENT_TOOL_IDS["usage.time.get"]
          ? result.state
          : result.applied,
      );
      requireValue(
        uint32(state.limit_seconds) &&
          uint32(state.used_seconds) &&
          state.exhausted ===
            (Number(state.limit_seconds) > 0 &&
              Number(state.used_seconds) >= Number(state.limit_seconds)),
      );
      break;
    }
    case CLIENT_TOOL_IDS["management.password.get"]:
      requireValue(typeof result.is_set === "boolean");
      break;
    case CLIENT_TOOL_IDS["management.password.set"]:
      requireValue(result.is_set === true);
      break;
    case CLIENT_TOOL_IDS["management.password.reset"]:
      requireValue(result.is_set === false);
      break;
  }
}
