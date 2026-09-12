import type { AgentInputItem } from "@openai/agents-core";

/** Marks the system item that carries the summary of earlier turns. */
export const SUMMARY_PREFIX = "【较早对话的摘要】";

export type ContextOptions = {
  /** Token budget for the conversation history sent with each turn. */
  maxTokens: number;
  /** Most recent turns always kept verbatim. */
  keepTurns: number;
  /** Characters kept from each tool result of an older turn. */
  trimChars: number;
};

export type Compaction = {
  /** Tool results shortened in older turns. */
  trimmedResults: number;
  /** Turns folded into the summary; 0 when only trimming was needed. */
  summarizedTurns: number;
};

/**
 * A rough token estimate that needs no tokenizer: CJK characters count as one
 * token each and other text as one token per four characters. It errs high,
 * which keeps the history under the model's real limit.
 */
export function estimateTokens(value: unknown): number {
  const text = typeof value === "string" ? value : JSON.stringify(value);
  let cjk = 0;
  for (const char of text) {
    if (/[　-鿿가-힯＀-￯]/.test(char)) cjk++;
  }
  return cjk + Math.ceil((text.length - cjk) / 4);
}

/** Splits history into turns, each starting at a user message. */
export function splitTurns(history: AgentInputItem[]): {
  summary?: AgentInputItem;
  turns: AgentInputItem[][];
} {
  let summary: AgentInputItem | undefined;
  const turns: AgentInputItem[][] = [];
  for (const item of history) {
    if (isSummary(item)) {
      summary = item;
      continue;
    }
    if (isUserMessage(item) || turns.length === 0) turns.push([]);
    turns[turns.length - 1].push(item);
  }
  return { summary, turns };
}

export function summaryItem(text: string): AgentInputItem {
  return { role: "system", content: `${SUMMARY_PREFIX}\n${text.trim()}` };
}

/**
 * Keeps the history within budget before a new message is added. It shortens
 * tool results of older turns first, then of every turn; if that is not
 * enough it keeps as many recent turns (up to keepTurns) as fit beside a
 * summary and folds the rest into a rolling summary produced by summarize.
 * The summary is cut to a quarter of the budget, so the result plus the
 * incoming message fits unless the incoming message alone does not.
 */
export async function compactHistory(
  history: AgentInputItem[],
  incoming: string,
  options: ContextOptions,
  summarize: (transcript: string) => Promise<string>,
): Promise<{ history: AgentInputItem[]; compaction?: Compaction }> {
  const size = (items: AgentInputItem[]) =>
    items.length === 0 ? 0 : estimateTokens(items);
  const budget = options.maxTokens - estimateTokens(incoming);
  if (size(history) <= budget) return { history };

  const { summary, turns } = splitTurns(history);
  const withSummary = (list: AgentInputItem[][]) => [
    ...(summary ? [summary] : []),
    ...list.flat(),
  ];
  let trimmedResults = 0;
  const trimTurn = (turn: AgentInputItem[]) =>
    turn.map((item) => {
      const trimmed = trimToolResult(item, options.trimChars);
      if (trimmed !== item) trimmedResults++;
      return trimmed;
    });

  // The latest turn keeps its full results while trimming older ones is enough.
  const olderTrimmed = [
    ...turns.slice(0, -1).map(trimTurn),
    ...turns.slice(-1),
  ];
  if (size(withSummary(olderTrimmed)) <= budget) {
    return {
      history: withSummary(olderTrimmed),
      compaction: { trimmedResults, summarizedTurns: 0 },
    };
  }
  const trimmed = [
    ...olderTrimmed.slice(0, -1),
    ...olderTrimmed.slice(-1).map(trimTurn),
  ];
  if (size(withSummary(trimmed)) <= budget) {
    return {
      history: withSummary(trimmed),
      compaction: { trimmedResults, summarizedTurns: 0 },
    };
  }

  // Up to a quarter of the budget, never more than what the incoming message
  // leaves, is reserved for the summary; the recent turns kept verbatim share
  // the rest.
  const reserve = Math.max(
    0,
    Math.min(Math.floor(options.maxTokens / 4), budget),
  );
  let keep = Math.min(options.keepTurns, trimmed.length);
  while (keep > 0 && size(trimmed.slice(-keep).flat()) > budget - reserve) {
    keep--;
  }
  const folded = trimmed.slice(0, trimmed.length - keep);
  const recent = keep > 0 ? trimmed.slice(-keep) : [];
  const compaction = { trimmedResults, summarizedTurns: folded.length };
  // Without room for even a short summary, the folded turns are dropped.
  if (reserve - 1 < estimateTokens(summaryItem("…")) + SUMMARY_MIN_TOKENS) {
    return { history: recent.flat(), compaction };
  }
  const transcript = [
    summary ? contentText(summary) : "",
    ...folded.map(renderTurn),
  ]
    .filter(Boolean)
    .join("\n\n");
  const text = cutToTokens(await summarize(transcript), reserve);
  return { history: [summaryItem(text), ...recent.flat()], compaction };
}

// The shortest summary worth asking the model for.
const SUMMARY_MIN_TOKENS = 20;

// Measured as the serialized summary item, whose prefix and JSON escaping
// count too; one token is left for joining it with the kept turns.
function cutToTokens(text: string, tokens: number): string {
  const whole = text.trim();
  let cut = whole;
  while (cut !== "" && estimateTokens(summaryItem(`${cut}…`)) > tokens - 1) {
    cut = cut.slice(0, Math.floor(cut.length * 0.9));
  }
  return cut === whole ? cut : `${cut}…`;
}

function isSummary(item: AgentInputItem): boolean {
  return (
    "role" in item &&
    item.role === "system" &&
    typeof item.content === "string" &&
    item.content.startsWith(SUMMARY_PREFIX)
  );
}

function isUserMessage(item: AgentInputItem): boolean {
  return "role" in item && item.role === "user";
}

function trimToolResult(item: AgentInputItem, limit: number): AgentInputItem {
  if (item.type !== "function_call_result") return item;
  const output = item.output;
  const text =
    typeof output === "string"
      ? output
      : !Array.isArray(output) && output.type === "text"
        ? output.text
        : undefined;
  if (text === undefined || text.length <= limit) return item;
  const shortened = `${text.slice(0, limit)}…（已截断，原长 ${text.length} 字符）`;
  return {
    ...item,
    output:
      typeof output === "string"
        ? shortened
        : { type: "text", text: shortened },
  } as AgentInputItem;
}

function contentText(item: AgentInputItem): string {
  if (!("content" in item)) return "";
  const content = item.content;
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .map((part) =>
      typeof part === "object" &&
      part !== null &&
      "text" in part &&
      typeof part.text === "string"
        ? part.text
        : "",
    )
    .join("");
}

function renderTurn(turn: AgentInputItem[]): string {
  return turn
    .map((item) => {
      if ("role" in item && item.role === "user")
        return `用户：${contentText(item)}`;
      if ("role" in item && item.role === "assistant")
        return `助手：${contentText(item)}`;
      if (item.type === "function_call")
        return `助手调用 ${item.name}(${item.arguments})`;
      if (item.type === "function_call_result") {
        const output = item.output;
        const text =
          typeof output === "string"
            ? output
            : !Array.isArray(output) && output.type === "text"
              ? output.text
              : "";
        return `工具 ${item.name} 返回：${text}`;
      }
      return "";
    })
    .filter(Boolean)
    .join("\n");
}
