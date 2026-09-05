// Giztest scenario documents, loaded and validated for the subset of the
// `gizclaw.test/v1alpha1` contract this runner executes.
//
// `api/giztest` and the Go runner own the complete schema. This runner
// executes the `rpc`, `client_rpc`, `http` and `output` step kinds and rejects
// every other kind at validation time instead of skipping it silently.
import { readdir, readFile, stat } from "node:fs/promises";
import path from "node:path";
import { parse as parseYAML } from "yaml";

export const DOCUMENT_VERSION = "gizclaw.test/v1alpha1";
export const MAX_DOCUMENT_BYTES = 4 << 20;
export const DEFAULT_TASK_TIMEOUT_MS = 5 * 60 * 1000;

export const SUPPORTED_OPERATIONS = [
  "rpc",
  "client_rpc",
  "http",
  "output",
  "reconnect",
] as const;

export const ALL_OPERATIONS = [
  "rpc",
  "telemetry",
  "rpc_stream",
  "client_rpc",
  "http",
  "speech",
  "peer_stream",
  "output",
  "review_op",
  "barrier",
  "reconnect",
  "workspace_relay",
] as const;

export const CLIENT_RPC_METHODS = [
  "client.info.get",
  "client.identifiers.get",
  "client.tool.invoke",
  "client.device.status.get",
  "client.device.volume.set",
  "client.device.sound.play",
  "client.device.reboot",
  "client.device.audioplayer.get",
  "client.device.audioplayer.playlist.get",
  "client.device.audioplayer.playlist.set",
  "client.device.audioplayer.playlist.append",
  "client.device.audioplayer.play",
  "client.device.audioplayer.stop",
  "client.device.audioplayer.mode.set",
  "client.wifi.status.get",
  "client.wifi.saved.list",
  "client.wifi.saved.forget",
  "client.wifi.scan",
  "client.wifi.connect",
] as const;

// Methods this runner can install a provider for. `client.tool.invoke` needs
// the tool-serving surface the JavaScript device SDK does not expose.
export const SUPPORTED_CLIENT_RPC_METHODS = new Set<string>([
  "client.info.get",
  "client.identifiers.get",
  "client.device.status.get",
  "client.device.volume.set",
  "client.device.sound.play",
  "client.device.reboot",
  "client.device.audioplayer.get",
  "client.device.audioplayer.playlist.get",
  "client.device.audioplayer.playlist.set",
  "client.device.audioplayer.playlist.append",
  "client.device.audioplayer.play",
  "client.device.audioplayer.stop",
  "client.device.audioplayer.mode.set",
  "client.wifi.status.get",
  "client.wifi.saved.list",
  "client.wifi.saved.forget",
  "client.wifi.scan",
  "client.wifi.connect",
]);

export type Operation = (typeof ALL_OPERATIONS)[number];

export type VariableSpec = {
  direction: "input" | "output";
  type: "string" | "integer" | "number" | "boolean" | "object";
  value?: unknown;
  env?: string;
  generate?: "uuid" | "token" | "string";
  secret?: boolean;
};

export type ClientSpec = {
  identity: "ephemeral";
  connection: "webrtc";
  access_point: string;
  registration_token?: string;
};

export type Expectation = {
  equals?: unknown;
  present?: boolean;
  non_empty?: boolean;
  count?: number;
  contains?: string;
  contains_all?: string[];
  contains_any?: string[];
  not_contains?: string | string[];
  pattern?: string;
  minimum?: number;
  maximum?: number;
  min_length?: number;
  max_length?: number;
};

export type Step = {
  telemetry?: { frame: Record<string, unknown> };
  id: string;
  client?: string;
  timeout?: string;
  save_as?: string;
  capture?: Record<string, string>;
  expect?: Record<string, Expectation>;
  expect_error?: { code: number; message_contains?: string };
  rpc?: { method: string; request: unknown };
  client_rpc?: { method: string; response?: unknown; expect_calls?: number };
  http?: {
    method: "GET" | "POST" | "PUT" | "DELETE";
    path: string;
    headers?: Record<string, string>;
    body?: unknown;
    status?: number;
  };
  output?: { variable: string };
  reconnect?: { await_ms?: number };
};

export type GiztestDocument = {
  path: string;
  version: string;
  name: string;
  repeat: number;
  timeout?: string;
  clients: Record<string, ClientSpec>;
  variables: Record<string, VariableSpec>;
  steps: Step[];
  finally: Step[];
  report?: { redact?: string[] };
};

const NAME_PATTERN = /^[a-z0-9][a-z0-9._-]{0,127}$/u;
const STEP_ID_PATTERN = /^[A-Za-z_][A-Za-z0-9_-]{0,63}$/u;
const VARIABLE_PATTERN = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/u;
const CLIENT_PATTERN = /^[A-Za-z_][A-Za-z0-9_-]{0,63}$/u;
const REFERENCE_PATTERN = /\$\{([A-Za-z_][A-Za-z0-9_]*)\}/gu;

// UnsupportedStepError marks a document this runner declines to execute
// because it uses a step kind outside its supported subset.
export class UnsupportedStepError extends Error {
  readonly operation: string;

  constructor(documentPath: string, operation: string) {
    super(`unsupported step kind for this runner: ${operation}`);
    this.name = "UnsupportedStepError";
    this.operation = operation;
  }
}

export function stepOperation(step: Step): Operation | undefined {
  for (const operation of ALL_OPERATIONS) {
    if ((step as Record<string, unknown>)[operation] != null) {
      return operation;
    }
  }
  return undefined;
}

// operationNeedsClient mirrors the Go runner: every kind except barrier,
// output, review and workspace_relay runs against a declared client.
export function operationNeedsClient(operation: Operation): boolean {
  return (
    operation !== "barrier" &&
    operation !== "output" &&
    operation !== "review_op" &&
    operation !== "workspace_relay"
  );
}

export function parseDuration(value: string): number {
  const match =
    /^(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m)?(?:(\d+(?:\.\d+)?)s)?(?:(\d+(?:\.\d+)?)ms)?$/u.exec(
      value.trim(),
    );
  if (match == null || match.slice(1).every((part) => part === undefined)) {
    throw new Error(`invalid duration: ${value}`);
  }
  const [, hours, minutes, seconds, millis] = match;
  return (
    Number(hours ?? 0) * 3_600_000 +
    Number(minutes ?? 0) * 60_000 +
    Number(seconds ?? 0) * 1000 +
    Number(millis ?? 0)
  );
}

export function taskTimeoutMs(document: GiztestDocument): number {
  if (document.timeout == null || document.timeout === "") {
    return DEFAULT_TASK_TIMEOUT_MS;
  }
  const duration = parseDuration(document.timeout);
  if (duration <= 0) {
    throw new Error(`task timeout must be positive: ${document.timeout}`);
  }
  return duration;
}

// collectReferences finds every ${name} reference in a step, matching the Go
// runner's whole-step JSON scan.
export function collectReferences(step: Step): string[] {
  const found = new Set<string>();
  for (const match of JSON.stringify(step).matchAll(REFERENCE_PATTERN)) {
    found.add(match[1]!);
  }
  return [...found];
}

function fail(documentPath: string, message: string): never {
  throw new Error(`${documentPath}: ${message}`);
}

// validateUserStory enforces the four-line user story header the Go runner
// requires before any YAML content.
function validateUserStory(documentPath: string, text: string): void {
  const prefixes = ["User Story:", "As a ", "I want ", "So that "];
  const lines: string[] = [];
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (line === "") {
      continue;
    }
    if (!line.startsWith("#")) {
      break;
    }
    lines.push(line.replace(/^#+/u, "").trim());
    if (lines.length === prefixes.length) {
      break;
    }
  }
  if (lines.length < prefixes.length) {
    fail(documentPath, "user story header requires four comment lines");
  }
  for (const [index, prefix] of prefixes.entries()) {
    const line = lines[index]!;
    if (!line.startsWith(prefix)) {
      fail(
        documentPath,
        `user story line ${index + 1} must start with ${prefix}`,
      );
    }
    if (index > 0 && line.slice(prefix.length).trim() === "") {
      fail(documentPath, `user story line ${index + 1} is empty`);
    }
  }
}

function validateVariables(
  documentPath: string,
  variables: Record<string, VariableSpec>,
): void {
  for (const [name, spec] of Object.entries(variables)) {
    if (!VARIABLE_PATTERN.test(name)) {
      fail(documentPath, `invalid variable name: ${name}`);
    }
    if (spec.direction !== "input" && spec.direction !== "output") {
      fail(documentPath, `variable ${name} has an invalid direction`);
    }
    const sources = [spec.value, spec.env, spec.generate].filter(
      (source) => source !== undefined,
    );
    if (spec.direction === "input" && sources.length !== 1) {
      fail(
        documentPath,
        `input variable ${name} requires exactly one of value, env or generate`,
      );
    }
    if (spec.direction === "output" && sources.length !== 0) {
      fail(documentPath, `output variable ${name} must not declare a source`);
    }
    if (
      spec.type !== "string" &&
      spec.type !== "integer" &&
      spec.type !== "number" &&
      spec.type !== "boolean" &&
      spec.type !== "object"
    ) {
      // audio and binary variables belong to the speech and stream steps this
      // runner does not execute.
      throw new UnsupportedStepError(documentPath, `variable:${spec.type}`);
    }
  }
}

function validateStep(
  documentPath: string,
  step: Step,
  clients: Record<string, ClientSpec>,
  produced: Map<string, boolean>,
  variables: Record<string, VariableSpec>,
  ids: Set<string>,
): void {
  if (!STEP_ID_PATTERN.test(step.id)) {
    fail(documentPath, `invalid step id: ${step.id}`);
  }
  if (ids.has(step.id)) {
    fail(documentPath, `duplicate step id: ${step.id}`);
  }
  ids.add(step.id);

  const operation = stepOperation(step);
  if (operation == null) {
    fail(documentPath, `step ${step.id} declares no operation`);
  }
  if (!(SUPPORTED_OPERATIONS as readonly string[]).includes(operation)) {
    throw new UnsupportedStepError(documentPath, operation);
  }
  if (operationNeedsClient(operation)) {
    if (step.client == null || clients[step.client] == null) {
      fail(documentPath, `step ${step.id} names an unknown client`);
    }
  }
  if (step.timeout != null && parseDuration(step.timeout) <= 0) {
    fail(documentPath, `step ${step.id} timeout must be positive`);
  }
  if (step.client_rpc != null) {
    const method = step.client_rpc.method;
    if (!(CLIENT_RPC_METHODS as readonly string[]).includes(method)) {
      fail(documentPath, `step ${step.id} uses unknown client RPC ${method}`);
    }
    if (!SUPPORTED_CLIENT_RPC_METHODS.has(method)) {
      throw new UnsupportedStepError(documentPath, `client_rpc:${method}`);
    }
    const calls = step.client_rpc.expect_calls;
    if (
      calls != null &&
      (!Number.isInteger(calls) || calls < 1 || calls > 1024)
    ) {
      fail(documentPath, `step ${step.id} expect_calls must be 1..1024`);
    }
  }
  if (step.http != null && !step.http.path.startsWith("/")) {
    fail(documentPath, `step ${step.id} http path must be absolute`);
  }

  for (const name of collectReferences(step)) {
    if (produced.get(name) !== true) {
      fail(
        documentPath,
        `step ${step.id} references unavailable variable ${name}`,
      );
    }
  }

  const targets = [
    ...(step.save_as == null ? [] : [step.save_as]),
    ...Object.keys(step.capture ?? {}),
  ];
  for (const target of targets) {
    const spec = variables[target];
    if (spec == null) {
      fail(documentPath, `step ${step.id} writes unknown variable ${target}`);
    }
    if (spec.direction !== "output") {
      fail(
        documentPath,
        `step ${step.id} writes non-output variable ${target}`,
      );
    }
    if (produced.get(target) === true) {
      fail(documentPath, `variable ${target} has more than one producer`);
    }
    produced.set(target, true);
  }
}

export function validateDocument(document: GiztestDocument): void {
  const documentPath = document.path;
  if (document.version !== DOCUMENT_VERSION) {
    fail(documentPath, `unsupported version: ${document.version}`);
  }
  if (!NAME_PATTERN.test(document.name)) {
    fail(documentPath, `invalid document name: ${document.name}`);
  }
  if (
    !Number.isInteger(document.repeat) ||
    document.repeat < 1 ||
    document.repeat > 1000
  ) {
    fail(documentPath, `repeat must be 1..1000`);
  }
  taskTimeoutMs(document);

  const clientNames = Object.keys(document.clients);
  if (clientNames.length === 0 || clientNames.length > 32) {
    fail(documentPath, "a document declares 1..32 clients");
  }
  for (const [name, spec] of Object.entries(document.clients)) {
    if (!CLIENT_PATTERN.test(name)) {
      fail(documentPath, `invalid client name: ${name}`);
    }
    if (spec.identity !== "ephemeral" || spec.connection !== "webrtc") {
      fail(documentPath, `client ${name} must be an ephemeral webrtc client`);
    }
    if (typeof spec.access_point !== "string" || spec.access_point === "") {
      fail(documentPath, `client ${name} requires an access_point`);
    }
  }

  validateVariables(documentPath, document.variables);

  const produced = new Map<string, boolean>();
  for (const [name, spec] of Object.entries(document.variables)) {
    produced.set(name, spec.direction === "input");
  }

  if (document.steps.length === 0 || document.steps.length > 512) {
    fail(documentPath, "a document declares 1..512 steps");
  }
  const ids = new Set<string>();
  for (const step of document.steps) {
    validateStep(
      documentPath,
      step,
      document.clients,
      produced,
      document.variables,
      ids,
    );
  }
  for (const step of document.finally) {
    if (step.client_rpc != null) {
      fail(documentPath, `finally step ${step.id} cannot install a client RPC`);
    }
    validateStep(
      documentPath,
      step,
      document.clients,
      produced,
      document.variables,
      ids,
    );
  }
  for (const name of document.report?.redact ?? []) {
    if (document.variables[name] == null) {
      fail(documentPath, `report.redact names unknown variable ${name}`);
    }
  }
}

export async function loadDocument(filePath: string): Promise<GiztestDocument> {
  const info = await stat(filePath);
  if (info.size > MAX_DOCUMENT_BYTES) {
    fail(filePath, `document exceeds ${MAX_DOCUMENT_BYTES} bytes`);
  }
  const text = await readFile(filePath, "utf8");
  validateUserStory(filePath, text);
  const parsed = parseYAML(text) as Partial<GiztestDocument> | null;
  if (parsed == null || typeof parsed !== "object") {
    fail(filePath, "document is not a YAML mapping");
  }
  const finalizers = (parsed.finally ?? []).map((step, index) => ({
    ...step,
    id: step.id == null || step.id === "" ? `finally_${index + 1}` : step.id,
  }));
  const document: GiztestDocument = {
    clients: parsed.clients ?? {},
    finally: finalizers,
    name: parsed.name ?? "",
    path: filePath,
    repeat: parsed.repeat ?? 1,
    report: parsed.report,
    steps: parsed.steps ?? [],
    timeout: parsed.timeout,
    variables: parsed.variables ?? {},
    version: parsed.version ?? "",
  };
  validateDocument(document);
  return document;
}

// discover expands files and directories into sorted, deduplicated scenario
// paths. Symbolic links are rejected, matching the Go runner.
export async function discover(inputs: string[]): Promise<string[]> {
  if (inputs.length === 0) {
    throw new Error("at least one file or directory is required");
  }
  const found = new Set<string>();
  for (const input of inputs) {
    const absolute = path.resolve(input);
    const info = await stat(absolute).catch(() => undefined);
    if (info == null) {
      throw new Error(`cannot read ${absolute}`);
    }
    const link = await import("node:fs/promises").then((fs) =>
      fs.lstat(absolute),
    );
    if (link.isSymbolicLink()) {
      throw new Error(`symlink is not allowed: ${absolute}`);
    }
    if (info.isDirectory()) {
      for (const entry of await readdir(absolute, {
        recursive: true,
        withFileTypes: true,
      })) {
        if (!entry.isFile() || !entry.name.endsWith(".giztest.yaml")) {
          continue;
        }
        found.add(path.join(entry.parentPath, entry.name));
      }
      continue;
    }
    if (!absolute.endsWith(".giztest.yaml")) {
      throw new Error(`not a .giztest.yaml file: ${absolute}`);
    }
    found.add(absolute);
  }
  if (found.size === 0) {
    throw new Error("no .giztest.yaml files selected");
  }
  return [...found].sort();
}

export async function loadDocuments(paths: string[]): Promise<{
  documents: GiztestDocument[];
  skipped: { path: string; reason: string }[];
}> {
  const documents: GiztestDocument[] = [];
  const skipped: { path: string; reason: string }[] = [];
  const names = new Set<string>();
  for (const filePath of paths) {
    let document: GiztestDocument;
    try {
      document = await loadDocument(filePath);
    } catch (error) {
      if (error instanceof UnsupportedStepError) {
        skipped.push({ path: filePath, reason: error.message });
        continue;
      }
      throw error;
    }
    if (names.has(document.name)) {
      fail(filePath, `duplicate document name: ${document.name}`);
    }
    names.add(document.name);
    documents.push(document);
  }
  documents.sort((left, right) => (left.path < right.path ? -1 : 1));
  return { documents, skipped };
}
