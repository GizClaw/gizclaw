import assert from "node:assert/strict";
import { test } from "node:test";

import { AssistantTurnError, createAssistant } from "../src/assistant.ts";
import { FakeRuntime } from "../testing/fake-runtime.ts";
import { runScenario } from "../testing/run-scenario.ts";
import { LIVING_ROOM, scenarios } from "../testing/scenarios.ts";
import { ScriptedModel } from "../testing/scripted-model.ts";

for (const scenario of scenarios) {
  test(`scenario ${scenario.name} runs through the Agents SDK runner`, async () => {
    const model = new ScriptedModel(scenario.script);
    const result = await runScenario(scenario, model);
    assert.deepEqual(result.failures, []);
    assert.ok(model.exhausted, "every scripted step was used");
    // Every scripted tool call reached the tool layer (parallel calls of one
    // step run concurrently, so order is not asserted), and every tool result
    // was handed back to the model on the following call.
    const scripted = scenario.script.flatMap((step) =>
      "call" in step ? step.call : [],
    );
    assert.deepEqual(
      result.actions.map((action) => action.tool).sort(),
      scripted.map((call) => call.name).sort(),
    );
    const lastInput = model.requests.at(-1)?.input;
    assert.ok(Array.isArray(lastInput));
    const outputs = lastInput.filter(
      (item) => item.type === "function_call_result",
    );
    assert.equal(outputs.length, scripted.length);
  });
}

test("history carries over to the next turn and reset clears it", async () => {
  const model = new ScriptedModel([
    { reply: "第一轮" },
    { reply: "第二轮" },
    { reply: "重来" },
  ]);
  const assistant = createAssistant({
    runtime: new FakeRuntime(scenarios[0].world),
    model,
  });
  assert.deepEqual(await assistant.send("你好"), {
    reply: "第一轮",
    actions: [],
  });
  await assistant.send("继续");
  const second = model.requests[1].input;
  assert.ok(Array.isArray(second));
  assert.deepEqual(
    second.map((item) => ("role" in item ? item.role : item.type)),
    ["user", "assistant", "user"],
  );
  assistant.reset();
  await assistant.send("重新开始");
  const third = model.requests[2].input;
  assert.ok(Array.isArray(third));
  assert.equal(third.length, 1);
});

test("the reply keeps text said alongside a tool call", async () => {
  const runtime = new FakeRuntime(scenarios[0].world);
  const assistant = createAssistant({
    runtime,
    model: new ScriptedModel([
      {
        say: "北京 Edge 最忙：512 个连接。",
        call: [
          {
            name: "navigate",
            arguments: { page: "server", node_id: "edge-bj" },
          },
        ],
      },
      { reply: "已跳转到它的详情页。" },
    ]),
  });
  const turn = await assistant.send("哪个节点最忙？");
  assert.equal(
    turn.reply,
    "北京 Edge 最忙：512 个连接。\n\n已跳转到它的详情页。",
  );
  assert.deepEqual(runtime.currentRoute, { page: "server", id: "edge-bj" });
});

test("a failed turn keeps the actions it already took", async () => {
  const assistant = createAssistant({
    runtime: new FakeRuntime(scenarios[1].world),
    model: new ScriptedModel([
      { call: [{ name: "get_current_page", arguments: {} }] },
    ]),
  });
  await assert.rejects(assistant.send("看看这个设备"), (error: unknown) => {
    assert.ok(error instanceof AssistantTurnError);
    assert.deepEqual(
      error.actions.map((action) => action.tool),
      ["get_current_page"],
    );
    return true;
  });
});

test("an aborted turn rejects without further tool calls", async () => {
  const runtime = new FakeRuntime(scenarios[1].world);
  const controller = new AbortController();
  controller.abort();
  const assistant = createAssistant({
    runtime,
    model: new ScriptedModel([
      {
        call: [{ name: "search_logs", arguments: { public_key: LIVING_ROOM } }],
      },
    ]),
  });
  await assert.rejects(
    assistant.send("查日志", { signal: controller.signal }),
    AssistantTurnError,
  );
  assert.equal(runtime.called("logs.search").length, 0);
});
