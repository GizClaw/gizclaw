import assert from "node:assert/strict";
import { test } from "node:test";

import type { AgentInputItem } from "@openai/agents-core";

import {
  compactHistory,
  estimateTokens,
  splitTurns,
  SUMMARY_PREFIX,
  summaryItem,
  type ContextOptions,
} from "../src/context.ts";

const options: ContextOptions = { maxTokens: 400, keepTurns: 2, trimChars: 40 };

function turn(index: number, resultChars = 10): AgentInputItem[] {
  return [
    { role: "user", content: `问题 ${index}` },
    {
      type: "function_call",
      callId: `c${index}`,
      name: "search_logs",
      arguments: "{}",
      status: "completed",
    },
    {
      type: "function_call_result",
      callId: `c${index}`,
      name: "search_logs",
      status: "completed",
      output: { type: "text", text: "x".repeat(resultChars) },
    },
    {
      type: "message",
      role: "assistant",
      status: "completed",
      content: [{ type: "output_text", text: `回答 ${index}` }],
    },
  ];
}

test("estimateTokens counts CJK per character and other text per four", () => {
  assert.equal(estimateTokens("调试模式"), 4);
  assert.equal(estimateTokens("abcdefgh"), 2);
  assert.equal(estimateTokens("调试abcd"), 3);
});

test("splitTurns separates the summary and starts turns at user messages", () => {
  const history = [summaryItem("早先"), ...turn(1), ...turn(2)];
  const split = splitTurns(history);
  assert.ok(split.summary);
  assert.equal(split.turns.length, 2);
  assert.equal(split.turns[1].length, 4);
});

test("a history within budget is left untouched", async () => {
  const history = turn(1);
  const result = await compactHistory(
    history,
    "下一个问题",
    options,
    async () => {
      throw new Error("must not summarize");
    },
  );
  assert.equal(result.history, history);
  assert.equal(result.compaction, undefined);
});

test("older tool results are trimmed before anything is summarized", async () => {
  const history = [...turn(1, 1_200), ...turn(2, 10)];
  const result = await compactHistory(history, "q", options, async () => {
    throw new Error("must not summarize");
  });
  assert.deepEqual(result.compaction, {
    trimmedResults: 1,
    summarizedTurns: 0,
  });
  const trimmed = result.history[2] as { output: { text: string } };
  assert.match(trimmed.output.text, /^x{40}…（已截断，原长 1200 字符）$/);
  // The latest turn keeps its full result.
  assert.equal(
    (result.history[6] as { output: { text: string } }).output.text,
    "x".repeat(10),
  );
});

test("older turns fold into a rolling summary, keeping the recent ones", async () => {
  const history = [
    summaryItem("用户关注客厅音箱"),
    ...Array.from({ length: 6 }, (_, index) => turn(index + 1, 300)).flat(),
  ];
  let transcript = "";
  const result = await compactHistory(history, "q", options, async (text) => {
    transcript = text;
    return "客厅音箱 ASR_TIMEOUT 17 次";
  });
  assert.equal(result.compaction?.summarizedTurns, 4);
  assert.match(transcript, /用户关注客厅音箱/);
  assert.match(transcript, /用户：问题 1/);
  assert.match(transcript, /工具 search_logs 返回/);
  const [summary, ...rest] = result.history;
  assert.deepEqual(summary, {
    role: "system",
    content: `${SUMMARY_PREFIX}\n客厅音箱 ASR_TIMEOUT 17 次`,
  });
  assert.deepEqual(
    splitTurns(rest).turns.map(
      (item) => (item[0] as { content: string }).content,
    ),
    ["问题 5", "问题 6"],
  );
});
