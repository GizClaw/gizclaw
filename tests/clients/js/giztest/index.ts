// giztest: run GizClaw scenario documents with the JavaScript SDKs.
//
// The device peer comes from `@gizclaw/gizclaw` and every Public HTTP step
// goes through `@gizclaw/gizclaw-control`, so one scenario suite exercises
// both sides of the JavaScript surface against a live server.
//
//   node --experimental-strip-types index.ts validate -f <file-or-directory>...
//   node --experimental-strip-types index.ts run <file-or-directory>... \
//     [--parallel N] [--output report.json]
import { rename, writeFile } from "node:fs/promises";
import path from "node:path";

import { discover, loadDocuments } from "./document.ts";
import { runDocuments } from "./runner.ts";

const EXIT_VALIDATION = 2;
const EXIT_EXECUTION = 4;

class CommandError extends Error {
  readonly exitCode: number;

  constructor(message: string, exitCode: number) {
    super(message);
    this.name = "CommandError";
    this.exitCode = exitCode;
  }
}

type RunFlags = { inputs: string[]; output?: string; parallel: number };

function parseRunFlags(argv: string[]): RunFlags {
  const inputs: string[] = [];
  let parallel = 1;
  let output: string | undefined;
  for (let index = 0; index < argv.length; index++) {
    const argument = argv[index]!;
    if (argument === "--parallel") {
      parallel = Number.parseInt(argv[++index] ?? "", 10);
      continue;
    }
    if (argument === "--output") {
      output = argv[++index];
      continue;
    }
    if (argument.startsWith("--")) {
      throw new CommandError(`unknown flag: ${argument}`, EXIT_VALIDATION);
    }
    inputs.push(argument);
  }
  if (inputs.length === 0) {
    throw new CommandError(
      "run requires at least one file or directory",
      EXIT_VALIDATION,
    );
  }
  if (!Number.isInteger(parallel) || parallel < 1) {
    throw new CommandError("--parallel must be at least 1", EXIT_VALIDATION);
  }
  return { inputs, output, parallel };
}

function parseValidateFlags(argv: string[]): string[] {
  const inputs: string[] = [];
  for (let index = 0; index < argv.length; index++) {
    const argument = argv[index]!;
    if (argument === "--file" || argument === "-f") {
      const value = argv[++index];
      if (value == null) {
        throw new CommandError("--file requires a value", EXIT_VALIDATION);
      }
      inputs.push(...value.split(","));
      continue;
    }
    throw new CommandError(
      "validate accepts no positional arguments",
      EXIT_VALIDATION,
    );
  }
  if (inputs.length === 0) {
    throw new CommandError("validate requires --file", EXIT_VALIDATION);
  }
  return inputs;
}

// writeReport replaces the target atomically so a partial file is never left
// behind for the caller to read.
async function writeReport(target: string, report: unknown): Promise<void> {
  const temporary = path.join(
    path.dirname(target),
    `.giztest-report-${process.pid}.json`,
  );
  await writeFile(temporary, `${JSON.stringify(report, undefined, 2)}\n`);
  await rename(temporary, target);
}

async function main(argv: string[]): Promise<number> {
  const [command, ...rest] = argv;
  if (command === "validate") {
    const paths = await discover(parseValidateFlags(rest));
    const { documents, skipped } = await loadDocuments(paths);
    for (const entry of skipped) {
      process.stderr.write(`skipped ${entry.path}: ${entry.reason}\n`);
    }
    process.stdout.write(`validated ${documents.length} Giztest documents\n`);
    return 0;
  }
  if (command === "run") {
    const flags = parseRunFlags(rest);
    const paths = await discover(flags.inputs);
    const { documents, skipped } = await loadDocuments(paths);
    for (const entry of skipped) {
      process.stderr.write(`skipped ${entry.path}: ${entry.reason}\n`);
    }
    const report = await runDocuments(documents, { parallel: flags.parallel });
    if (flags.output != null) {
      await writeReport(flags.output, report);
    }
    process.stdout.write(
      `Giztest ${report.status}: ${report.tasks.length} tasks in ${report.duration_ms}ms\n`,
    );
    return report.status === "passed" ? 0 : EXIT_EXECUTION;
  }
  throw new CommandError("usage: giztest <validate|run> ...", EXIT_VALIDATION);
}

main(process.argv.slice(2))
  .then((code) => process.exit(code))
  .catch((error: unknown) => {
    process.stderr.write(
      `${error instanceof Error ? error.message : String(error)}\n`,
    );
    process.exit(error instanceof CommandError ? error.exitCode : 1);
  });
