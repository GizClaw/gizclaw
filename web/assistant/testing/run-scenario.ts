import type { Model } from "@openai/agents-core";

import { createAssistant } from "../src/assistant.ts";
import type { ActionRecord } from "../src/tools.ts";
import { evaluate } from "./expectations.ts";
import { FakeRuntime } from "./fake-runtime.ts";
import { SCENARIO_NOW, type Scenario } from "./scenarios.ts";

export type ScenarioResult = {
  scenario: string;
  passed: boolean;
  failures: string[];
  reply: string;
  actions: ActionRecord[];
  runtime: FakeRuntime;
};

/** Runs every turn of a scenario against a fresh FakeRuntime and evaluates it. */
export async function runScenario(
  scenario: Scenario,
  model: Model,
  options: { signal?: AbortSignal } = {},
): Promise<ScenarioResult> {
  const runtime = new FakeRuntime(scenario.world);
  const assistant = createAssistant({
    runtime,
    model,
    now: () => SCENARIO_NOW,
  });
  const actions: ActionRecord[] = [];
  let reply = "";
  for (const message of scenario.turns) {
    const turn = await assistant.send(message, options);
    actions.push(...turn.actions);
    reply = turn.reply;
  }
  const failures = evaluate(scenario.expect, {
    reply,
    actions,
    route: runtime.currentRoute,
  });
  return {
    scenario: scenario.name,
    passed: failures.length === 0,
    failures,
    reply,
    actions,
    runtime,
  };
}
