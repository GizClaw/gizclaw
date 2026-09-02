// JSON-pointer resolution and expectation matching for scenario steps.
//
// Assertion messages never echo the asserted content, matching the Go runner:
// a failure names the pointer and the operator only.
import type { Expectation } from "./document.ts";

export type PointerResult = { found: boolean; value: unknown };

// AssertionFailure marks an expectation or declared-status failure, which the
// report distinguishes from an operation failure.
export class AssertionFailure extends Error {
  constructor(message: string) {
    super(message);
    this.name = "AssertionFailure";
  }
}

export function jsonPointer(input: unknown, pointer: string): PointerResult {
  if (pointer === "") {
    return { found: true, value: input };
  }
  let current = input;
  for (const raw of pointer.replace(/^\//u, "").split("/")) {
    const part = raw.replaceAll("~1", "/").replaceAll("~0", "~");
    if (Array.isArray(current)) {
      const index = Number.parseInt(part, 10);
      if (!Number.isInteger(index) || index < 0 || index >= current.length) {
        return { found: false, value: undefined };
      }
      current = current[index];
      continue;
    }
    if (current != null && typeof current === "object") {
      const record = current as Record<string, unknown>;
      if (!Object.hasOwn(record, part)) {
        return { found: false, value: undefined };
      }
      current = record[part];
      continue;
    }
    return { found: false, value: undefined };
  }
  return { found: true, value: current };
}

// jsonEqual compares two values by their canonical JSON text, so numbers and
// strings never compare equal and object key order does not matter.
export function jsonEqual(left: unknown, right: unknown): boolean {
  return stableJSON(left) === stableJSON(right);
}

function stableJSON(value: unknown): string {
  if (value === undefined) {
    return "null";
  }
  if (Array.isArray(value)) {
    return `[${value.map((item) => stableJSON(item)).join(",")}]`;
  }
  if (value != null && typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>)
      .filter(([, item]) => item !== undefined)
      .sort(([left], [right]) => (left < right ? -1 : 1));
    return `{${entries.map(([key, item]) => `${JSON.stringify(key)}:${stableJSON(item)}`).join(",")}}`;
  }
  return JSON.stringify(value) ?? "null";
}

// stringTarget coerces a value to matchable text: a string, or an array of
// strings joined without a separator.
function stringTarget(value: unknown): string | undefined {
  if (typeof value === "string") {
    return value;
  }
  if (Array.isArray(value)) {
    let joined = "";
    for (const item of value) {
      if (typeof item !== "string") {
        return undefined;
      }
      joined += item;
    }
    return joined;
  }
  return undefined;
}

// numericTarget accepts numbers and the decimal strings protojson emits for
// 64-bit integer fields.
function numericTarget(value: unknown): number | undefined {
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : undefined;
  }
  if (typeof value === "string") {
    const parsed = Number.parseFloat(value.trim());
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

function isEmpty(value: unknown): boolean {
  if (value == null) {
    return true;
  }
  if (typeof value === "string") {
    return value === "";
  }
  if (Array.isArray(value)) {
    return value.length === 0;
  }
  if (typeof value === "object") {
    return Object.keys(value as Record<string, unknown>).length === 0;
  }
  return false;
}

function assertStringMatchers(
  path: string,
  value: unknown,
  expectation: Expectation,
): void {
  const needles = [
    expectation.contains,
    expectation.contains_all,
    expectation.contains_any,
    expectation.not_contains,
    expectation.pattern,
    expectation.min_length,
    expectation.max_length,
  ];
  if (needles.every((needle) => needle === undefined)) {
    return;
  }
  const text = stringTarget(value);
  if (text === undefined) {
    throw new AssertionFailure(
      `assert ${path} requires a string or text-fragment target`,
    );
  }
  if (expectation.contains != null && !text.includes(expectation.contains)) {
    throw new AssertionFailure(`assert ${path} contains failed`);
  }
  if (
    expectation.contains_all != null &&
    !expectation.contains_all.every((needle) => text.includes(needle))
  ) {
    throw new AssertionFailure(`assert ${path} contains_all failed`);
  }
  if (
    expectation.contains_any != null &&
    !expectation.contains_any.some((needle) => text.includes(needle))
  ) {
    throw new AssertionFailure(`assert ${path} contains_any failed`);
  }
  if (expectation.not_contains != null) {
    const forbidden =
      typeof expectation.not_contains === "string"
        ? [expectation.not_contains]
        : expectation.not_contains;
    if (forbidden.some((needle) => text.includes(needle))) {
      throw new AssertionFailure(`assert ${path} not_contains failed`);
    }
  }
  if (
    expectation.pattern != null &&
    !new RegExp(expectation.pattern, "u").test(text)
  ) {
    throw new AssertionFailure(`assert ${path} pattern failed`);
  }
  const length = [...text].length;
  if (expectation.min_length != null && length < expectation.min_length) {
    throw new AssertionFailure(`assert ${path} min_length failed`);
  }
  if (expectation.max_length != null && length > expectation.max_length) {
    throw new AssertionFailure(`assert ${path} max_length failed`);
  }
}

function assertNumericBounds(
  path: string,
  value: unknown,
  expectation: Expectation,
): void {
  if (expectation.minimum == null && expectation.maximum == null) {
    return;
  }
  const number = numericTarget(value);
  if (number === undefined) {
    throw new AssertionFailure(`assert ${path} requires a numeric target`);
  }
  if (expectation.minimum != null && number < expectation.minimum) {
    throw new AssertionFailure(`assert ${path} minimum failed`);
  }
  if (expectation.maximum != null && number > expectation.maximum) {
    throw new AssertionFailure(`assert ${path} maximum failed`);
  }
}

export function assertValue(
  input: unknown,
  expectations: Record<string, Expectation>,
): void {
  for (const [path, expectation] of Object.entries(expectations)) {
    const { found, value } = jsonPointer(input, path);
    if (expectation.present != null && found !== expectation.present) {
      // The Go runner reports the observed presence, not the expected one.
      throw new AssertionFailure(`assert ${path} presence = ${found}`);
    }
    if (!found) {
      if (expectation.present === false) {
        continue;
      }
      throw new AssertionFailure(`assert path ${path} not found`);
    }
    if (
      expectation.equals !== undefined &&
      !jsonEqual(value, expectation.equals)
    ) {
      throw new AssertionFailure(`assert ${path} equals failed`);
    }
    if (expectation.count != null) {
      if (!Array.isArray(value) || value.length !== expectation.count) {
        throw new AssertionFailure(`assert ${path} count failed`);
      }
    }
    if (
      expectation.non_empty != null &&
      isEmpty(value) === expectation.non_empty
    ) {
      throw new AssertionFailure(`assert ${path} non_empty failed`);
    }
    assertStringMatchers(path, value, expectation);
    assertNumericBounds(path, value, expectation);
  }
}

// safeError redacts secrets, blanks any message naming a credential concept,
// and caps the length, matching the Go runner's report sanitation.
export function safeError(error: unknown, redactions: string[] = []): string {
  if (error == null) {
    return "";
  }
  let text = error instanceof Error ? error.message : String(error);
  for (const secret of redactions) {
    if (secret !== "") {
      text = text.replaceAll(secret, "[REDACTED]");
    }
  }
  const lowered = text.toLowerCase();
  for (const word of ["token", "credential", "authorization", "private_key"]) {
    if (lowered.includes(word)) {
      return "redacted execution error";
    }
  }
  return text.length > 512 ? text.slice(0, 512) : text;
}
