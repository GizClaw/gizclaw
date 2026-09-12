// Runs every scenario against a real GizClaw /openai/v1 model. The model's
// wording varies between runs, so each scenario gets two retries and is judged
// only on tool calls, the final route and required facts.
//
//   GIZCLAW_ASSISTANT_BASE_URL  https://<node>/openai/v1
//   GIZCLAW_ASSISTANT_API_KEY   a GizClaw API key
//   GIZCLAW_ASSISTANT_MODEL     RuntimeProfile model alias (default "llm")
//   GIZCLAW_ASSISTANT_REPORT    optional JSON report path

import { writeFile } from "node:fs/promises";

import { createGizClawModel } from "../src/model.ts";
import { runScenario } from "../testing/run-scenario.ts";
import { scenarios } from "../testing/scenarios.ts";

const ATTEMPTS = 3;
const TURN_TIMEOUT_MS = 120_000;

type AttemptReport = {
  passed: boolean;
  failures: string[];
  reply: string;
  actions: unknown[];
  elapsed_ms: number;
};

function required(name: string): string {
  const value = process.env[name]?.trim();
  if (!value) throw new Error(`${name} is required`);
  return value;
}

async function main(): Promise<number> {
  const model = createGizClawModel({
    baseURL: required("GIZCLAW_ASSISTANT_BASE_URL"),
    apiKey: required("GIZCLAW_ASSISTANT_API_KEY"),
    model: process.env.GIZCLAW_ASSISTANT_MODEL?.trim() || "llm",
  });
  const report: {
    scenario: string;
    passed: boolean;
    attempts: AttemptReport[];
  }[] = [];
  for (const scenario of scenarios) {
    const attempts: AttemptReport[] = [];
    for (let attempt = 1; attempt <= ATTEMPTS; attempt++) {
      const started = Date.now();
      try {
        const result = await runScenario(scenario, model, {
          signal: AbortSignal.timeout(TURN_TIMEOUT_MS),
        });
        attempts.push({
          passed: result.passed,
          failures: result.failures,
          reply: result.reply,
          actions: result.actions,
          elapsed_ms: Date.now() - started,
        });
      } catch (error) {
        attempts.push({
          passed: false,
          failures: [
            `turn failed: ${error instanceof Error ? error.message : String(error)}`,
          ],
          reply: "",
          actions: (error as { actions?: unknown[] }).actions ?? [],
          elapsed_ms: Date.now() - started,
        });
      }
      const last = attempts.at(-1)!;
      console.log(
        `${last.passed ? "PASS" : "FAIL"} ${scenario.name} attempt ${attempt} (${last.elapsed_ms} ms)`,
      );
      for (const failure of last.failures) console.log(`  - ${failure}`);
      if (last.passed) break;
    }
    report.push({
      scenario: scenario.name,
      passed: attempts.some((item) => item.passed),
      attempts,
    });
  }
  const path = process.env.GIZCLAW_ASSISTANT_REPORT?.trim();
  if (path) await writeFile(path, `${JSON.stringify(report, null, 2)}\n`);
  const failed = report
    .filter((item) => !item.passed)
    .map((item) => item.scenario);
  console.log(
    failed.length === 0
      ? "all scenarios passed"
      : `failed scenarios: ${failed.join(", ")}`,
  );
  return failed.length === 0 ? 0 : 1;
}

process.exitCode = await main();
