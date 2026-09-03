// Scenario execution: one task per document repeat, steps in order, then the
// finalizers, producing the same report shape as the Go runner.
import {
  AssertionFailure,
  assertValue,
  jsonPointer,
  safeError,
} from "./assert.ts";
import {
  type GiztestDocument,
  type Step,
  stepOperation,
  parseDuration,
  taskTimeoutMs,
} from "./document.ts";
import { ScenarioClient } from "./client.ts";
import { Variables } from "./variables.ts";

const CLEANUP_BUDGET_MS = 30_000;
const CLIENT_RPC_POLL_MS = 10;

export type StepReport = {
  id: string;
  operation: string;
  client?: string;
  status: "passed" | "failed";
  stage: string;
  duration_ms: number;
  error?: string;
  evidence?: Record<string, unknown>;
};

export type TaskReport = {
  path: string;
  name: string;
  task_id: string;
  status: "passed" | "failed";
  repeat_index: number;
  duration_ms: number;
  clients?: Record<string, string>;
  steps: StepReport[];
  cleanup?: StepReport[];
  error?: string;
};

export type Report = {
  version: "v1";
  status: "passed" | "failed";
  started_at: string;
  duration_ms: number;
  tasks: TaskReport[];
};

export type RunOptions = {
  parallel: number;
  out?: NodeJS.WriteStream;
};

type StepOutcome = {
  evidence?: Record<string, unknown>;
  value?: unknown;
};

function taskID(document: GiztestDocument, index: number): string {
  return `${document.name}-${String(index).padStart(4, "0")}`;
}

function withTimeout(signal: AbortSignal, timeout?: string): AbortSignal {
  if (timeout == null || timeout === "") {
    return signal;
  }
  return AbortSignal.any([signal, AbortSignal.timeout(parseDuration(timeout))]);
}

// runStep executes one step and returns its result value and evidence. The
// value is what capture and expect resolve pointers against.
async function runStep(
  step: Step,
  clients: Map<string, ScenarioClient>,
  variables: Variables,
  signal: AbortSignal,
  out: NodeJS.WriteStream,
): Promise<StepOutcome> {
  const client = step.client == null ? undefined : clients.get(step.client);

  if (step.rpc != null) {
    if (client == null) {
      throw new Error(`step ${step.id} has no connected client`);
    }
    const params = variables.resolve(step.rpc.request ?? {});
    const value = await client.callRPC(step.rpc.method, params, signal);
    return { value };
  }

  if (step.http != null) {
    if (client == null) {
      throw new Error(`step ${step.id} has no connected client`);
    }
    const path = variables.resolveString(step.http.path, "http path");
    if (!path.startsWith("/")) {
      throw new Error("http path must resolve to an absolute path");
    }
    const headers: Record<string, string> = {};
    for (const [name, raw] of Object.entries(step.http.headers ?? {})) {
      headers[name] = variables.resolveString(raw, `header ${name}`);
    }
    const body =
      step.http.body === undefined
        ? undefined
        : variables.resolve(step.http.body);
    const result = await client.callHTTP(
      step.http.method,
      path,
      headers,
      body,
      signal,
    );
    const evidence = {
      method: step.http.method,
      path,
      status: result.status,
    };
    const declared = step.http.status;
    if (declared != null && result.status !== declared) {
      throw new AssertionFailure(
        `http status = ${result.status}, want ${declared}`,
      );
    }
    if (declared == null && result.status >= 400) {
      throw new AssertionFailure(`http status = ${result.status}`);
    }
    return { evidence, value: result.body };
  }

  if (step.client_rpc != null) {
    if (client == null) {
      throw new Error(`step ${step.id} has no connected client`);
    }
    const method = step.client_rpc.method;
    if (!client.inbound.has(method)) {
      throw new Error(`client RPC ${method} was not installed`);
    }
    const expected = step.client_rpc.expect_calls ?? 1;
    // The counter is cumulative from connect time and the wait is "at least
    // N", matching the Go runner.
    while ((client.inbound.get(method) ?? 0) < expected) {
      if (signal.aborted) {
        throw new Error(
          `client RPC ${method} calls = ${client.inbound.get(method) ?? 0}, want at least ${expected}`,
        );
      }
      await new Promise((resolve) => setTimeout(resolve, CLIENT_RPC_POLL_MS));
    }
    const evidence = { calls: client.inbound.get(method) ?? 0, method };
    return { evidence, value: evidence };
  }

  if (step.output != null) {
    const name = step.output.variable;
    const entry = variables.get(name);
    if (entry == null || entry.data == null) {
      throw new Error(`output variable ${name} unavailable`);
    }
    if (entry.spec.secret) {
      throw new Error(`secret variable ${name} cannot be emitted`);
    }
    if (entry.spec.type === "object") {
      throw new Error(`variable ${name} type object cannot be emitted`);
    }
    const text = String(entry.data);
    out.write(`${name}=${text}\n`);
    return {
      evidence: {
        bytes: Buffer.byteLength(text),
        truncated: false,
        type: entry.spec.type,
        variable: name,
      },
    };
  }

  throw new Error(`step ${step.id} declares no supported operation`);
}

async function runStepReport(
  step: Step,
  clients: Map<string, ScenarioClient>,
  variables: Variables,
  signal: AbortSignal,
  redactions: string[],
  stage: string,
  out: NodeJS.WriteStream,
): Promise<{ failure?: Error; report: StepReport }> {
  const operation = stepOperation(step)!;
  const started = Date.now();
  const report: StepReport = {
    duration_ms: 0,
    id: step.id,
    operation,
    stage,
    status: "failed",
    ...(step.client == null ? {} : { client: step.client }),
  };
  let failure: Error | undefined;
  let outcome: StepOutcome = {};
  try {
    outcome = await runStep(
      step,
      clients,
      variables,
      withTimeout(signal, step.timeout),
      out,
    );
  } catch (error) {
    failure = error instanceof Error ? error : new Error(String(error));
  }

  if (step.expect_error != null) {
    const code = rpcErrorCode(failure);
    if (code == null) {
      if (failure == null) {
        failure = new AssertionFailure(
          `expected RPC error code ${step.expect_error.code}, got success`,
        );
      }
    } else if (code !== step.expect_error.code) {
      failure = new AssertionFailure(
        `RPC error code = ${code}, want ${step.expect_error.code}`,
      );
    } else if (
      step.expect_error.message_contains != null &&
      !(failure?.message ?? "").includes(step.expect_error.message_contains)
    ) {
      failure = new AssertionFailure(
        "RPC error message does not contain expected text",
      );
    } else {
      failure = undefined;
      outcome = { evidence: { rpc_error_code: code } };
    }
  }

  if (failure == null && outcome.value != null) {
    try {
      if (step.save_as != null) {
        variables.assign(step.save_as, outcome.value);
      }
      for (const [name, pointer] of Object.entries(step.capture ?? {})) {
        const { found, value } = jsonPointer(outcome.value, pointer);
        if (!found) {
          throw new Error(`capture pointer ${pointer} not found`);
        }
        variables.assign(name, value);
      }
      assertValue(outcome.value, step.expect ?? {});
    } catch (error) {
      failure = error instanceof Error ? error : new Error(String(error));
    }
  }

  report.duration_ms = Date.now() - started;
  report.status = failure == null ? "passed" : "failed";
  if (failure != null) {
    report.error = safeError(failure, redactions);
  }
  const evidence =
    outcome.evidence ??
    (failure == null && outcome.value != null
      ? { result: "captured" }
      : undefined);
  if (evidence != null && Object.keys(evidence).length > 0) {
    report.evidence = evidence;
  }
  return { failure, report };
}

function rpcErrorCode(error: Error | undefined): number | undefined {
  if (error == null) {
    return undefined;
  }
  const code = (error as { code?: unknown }).code;
  return typeof code === "number" ? code : undefined;
}

async function runTask(
  document: GiztestDocument,
  repeatIndex: number,
  out: NodeJS.WriteStream,
): Promise<TaskReport> {
  const started = Date.now();
  const report: TaskReport = {
    duration_ms: 0,
    name: document.name,
    path: document.path,
    repeat_index: repeatIndex,
    status: "failed",
    steps: [],
    task_id: taskID(document, repeatIndex),
  };

  let variables: Variables;
  try {
    variables = new Variables(document.variables);
  } catch (error) {
    report.error = safeError(error);
    report.duration_ms = Date.now() - started;
    return report;
  }
  const redactions = variables.redactions(document.report?.redact ?? []);
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), taskTimeoutMs(document));
  const clients = new Map<string, ScenarioClient>();
  const fingerprints: Record<string, string> = {};

  try {
    for (const name of Object.keys(document.clients).sort()) {
      // Connecting races the task budget so a multi-client document cannot
      // overrun its timeout before the first step runs.
      const client = await ScenarioClient.connect(
        name,
        document.clients[name]!,
        document.steps,
        variables,
        controller.signal,
      );
      clients.set(name, client);
      fingerprints[name] = client.fingerprint;
    }
    for (const step of document.steps) {
      const { failure, report: stepReport } = await runStepReport(
        step,
        clients,
        variables,
        controller.signal,
        redactions,
        stepOperation(step)!,
        out,
      );
      report.steps.push(stepReport);
      if (failure != null) {
        report.error = safeError(failure, redactions);
        break;
      }
    }
    report.status = report.error == null ? "passed" : "failed";
  } catch (error) {
    report.error = safeError(error, redactions);
    report.status = "failed";
  } finally {
    clearTimeout(timer);
  }

  if (Object.keys(fingerprints).length > 0) {
    report.clients = fingerprints;
  }

  // Finalizers always run, on a fresh budget detached from the task timeout.
  if (document.finally.length > 0) {
    const cleanup: StepReport[] = [];
    const cleanupController = new AbortController();
    const cleanupTimer = setTimeout(
      () => cleanupController.abort(),
      CLEANUP_BUDGET_MS,
    );
    for (const step of document.finally) {
      const { failure, report: stepReport } = await runStepReport(
        step,
        clients,
        variables,
        cleanupController.signal,
        redactions,
        "cleanup",
        out,
      );
      cleanup.push(stepReport);
      if (failure != null) {
        report.status = "failed";
        report.error ??= safeError(failure, redactions);
      }
    }
    clearTimeout(cleanupTimer);
    report.cleanup = cleanup;
  }

  for (const client of clients.values()) {
    client.close();
  }
  report.duration_ms = Date.now() - started;
  return report;
}

export async function runDocuments(
  documents: GiztestDocument[],
  options: RunOptions,
): Promise<Report> {
  const started = new Date();
  const out = options.out ?? process.stdout;
  const queue: { document: GiztestDocument; repeatIndex: number }[] = [];
  for (const document of documents) {
    for (let index = 0; index < document.repeat; index++) {
      queue.push({ document, repeatIndex: index });
    }
  }

  const tasks: TaskReport[] = [];
  let next = 0;
  const workers = Array.from(
    { length: Math.max(1, Math.min(options.parallel, queue.length || 1)) },
    async () => {
      for (;;) {
        const index = next++;
        if (index >= queue.length) {
          return;
        }
        const item = queue[index]!;
        tasks.push(await runTask(item.document, item.repeatIndex, out));
      }
    },
  );
  await Promise.all(workers);

  tasks.sort((left, right) =>
    left.path === right.path
      ? left.repeat_index - right.repeat_index
      : left.path < right.path
        ? -1
        : 1,
  );
  return {
    duration_ms: Date.now() - started.getTime(),
    started_at: started.toISOString(),
    status: tasks.every((task) => task.status === "passed")
      ? "passed"
      : "failed",
    tasks,
    version: "v1",
  };
}
