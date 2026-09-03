// Scenario variables: input resolution, ${name} substitution and single-write
// output capture, matching the Go runner's semantics.
import { randomBytes } from "node:crypto";

import type { VariableSpec } from "./document.ts";

type Entry = { data: unknown; spec: VariableSpec };

const ANCHORED_REFERENCE = /^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$/u;
const EMBEDDED_REFERENCE = /\$\{([A-Za-z_][A-Za-z0-9_]*)\}/gu;

// generateValue mirrors the Go runner exactly: a v4 UUID for `uuid`, and
// "g" followed by 32 lowercase hex characters for `token` and `string`.
export function generateValue(kind: string): string {
  const bytes = randomBytes(16);
  if (kind === "uuid") {
    bytes[6] = (bytes[6]! & 0x0f) | 0x40;
    bytes[8] = (bytes[8]! & 0x3f) | 0x80;
    const hex = bytes.toString("hex");
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  }
  return `g${bytes.toString("hex")}`;
}

function checkValueType(name: string, spec: VariableSpec, data: unknown): void {
  if (data == null) {
    if (spec.direction === "output") {
      return;
    }
    throw new Error(`variable ${name} is empty`);
  }
  switch (spec.type) {
    case "string":
      if (typeof data !== "string") {
        throw new Error(`variable ${name} want string`);
      }
      return;
    case "boolean":
      if (typeof data !== "boolean") {
        throw new Error(`variable ${name} want boolean`);
      }
      return;
    case "integer":
      if (typeof data !== "number" || !Number.isInteger(data)) {
        throw new Error(`variable ${name} want integer`);
      }
      return;
    case "number":
      if (typeof data !== "number") {
        throw new Error(`variable ${name} want number`);
      }
      return;
    case "object":
      if (typeof data !== "object" || Array.isArray(data)) {
        throw new Error(`variable ${name} want object`);
      }
      return;
    default:
      throw new Error(`variable ${name} has unsupported type ${spec.type}`);
  }
}

export class Variables {
  private readonly entries = new Map<string, Entry>();

  constructor(
    specs: Record<string, VariableSpec>,
    env: NodeJS.ProcessEnv = process.env,
  ) {
    for (const [name, spec] of Object.entries(specs)) {
      if (spec.direction === "output") {
        this.entries.set(name, { data: undefined, spec });
        continue;
      }
      let data: unknown;
      if (spec.env != null && spec.env !== "") {
        const value = env[spec.env];
        if (value === undefined) {
          throw new Error(
            `input variable ${name} requires environment ${spec.env}`,
          );
        }
        data = value;
      } else if (spec.generate != null) {
        data = generateValue(spec.generate);
      } else {
        data = spec.value;
      }
      checkValueType(name, spec, data);
      this.entries.set(name, { data, spec });
    }
  }

  // redactions lists the secret string values to mask in reported errors,
  // longest first so a longer secret is masked before one of its prefixes.
  redactions(requested: string[] = []): string[] {
    const explicit = new Set(requested);
    const found: string[] = [];
    for (const [name, entry] of this.entries) {
      if (!entry.spec.secret && !explicit.has(name)) {
        continue;
      }
      if (typeof entry.data === "string" && entry.data !== "") {
        found.push(entry.data);
      }
    }
    return found.sort((left, right) => right.length - left.length);
  }

  get(name: string): Entry | undefined {
    return this.entries.get(name);
  }

  assign(name: string, data: unknown): void {
    const current = this.entries.get(name);
    if (current == null) {
      throw new Error(`unknown variable ${name}`);
    }
    if (current.spec.direction !== "output") {
      throw new Error(`variable ${name} is not output`);
    }
    if (current.data != null) {
      throw new Error(`variable ${name} already assigned`);
    }
    checkValueType(name, current.spec, data);
    this.entries.set(name, { data, spec: current.spec });
  }

  // resolve substitutes ${name} references. A string that is exactly one
  // reference keeps the variable's runtime type; embedded references require
  // string values and produce a string.
  resolve(input: unknown): unknown {
    if (typeof input === "string") {
      const anchored = ANCHORED_REFERENCE.exec(input);
      if (anchored != null) {
        const entry = this.entries.get(anchored[1]!);
        if (entry == null || entry.data == null) {
          throw new Error(`variable ${anchored[1]} unavailable`);
        }
        return entry.data;
      }
      let result = input;
      for (const match of input.matchAll(EMBEDDED_REFERENCE)) {
        const name = match[1]!;
        const entry = this.entries.get(name);
        if (entry == null || entry.data == null) {
          throw new Error(`variable ${name} unavailable`);
        }
        if (typeof entry.data !== "string") {
          throw new Error(`embedded variable ${name} must be string`);
        }
        result = result.replaceAll(`\${${name}}`, entry.data);
      }
      return result;
    }
    if (Array.isArray(input)) {
      return input.map((item) => this.resolve(item));
    }
    if (input != null && typeof input === "object") {
      const resolved: Record<string, unknown> = {};
      for (const [key, value] of Object.entries(
        input as Record<string, unknown>,
      )) {
        resolved[key] = this.resolve(value);
      }
      return resolved;
    }
    return input;
  }

  resolveString(input: unknown, label: string): string {
    const value = this.resolve(input);
    if (typeof value !== "string") {
      throw new Error(`${label} must resolve to string`);
    }
    return value;
  }
}
