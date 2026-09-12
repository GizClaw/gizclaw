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
 * Keeps the history within budget before a new message is added: first it
 * shortens tool results of older turns, then it folds everything but the most
 * recent turns into a rolling summary produced by summarize.
 */
export async function compactHistory(
  history: AgentInputItem[],
  incoming: string,
  options: ContextOptions,
  summarize: (transcript: string) => Promise<string>,
): Promise<{ history: AgentInputItem[]; compaction?: Compaction }> {
  const fits = (items: AgentInputItem[]) =>
    estimateTokens(items) + estimateTokens(incoming) <= options.maxTokens;
  if (fits(history)) return { history };

  const { summary, turns } = splitTurns(history);
  let trimmedResults = 0;
  const trimTurns = (list: AgentInputItem[][]) =>
    list.map((turn) =>
      turn.map((item) => {
        const trimmed = trimToolResult(item, options.trimChars);
        if (trimmed !== item) trimmedResults++;
        return trimmed;
      }),
    );

  const older = turns.slice(0, -1);
  const last = turns.slice(-1);
  const trimmed = [...trimTurns(older), ...last];
  const afterTrim = [...(summary ? [summary] : []), ...trimmed.flat()];
  if (fits(afterTrim)) {
    return {
      history: afterTrim,
      compaction: { trimmedResults, summarizedTurns: 0 },
    };
  }

  if (trimmed.length <= options.keepTurns) {
    // Only the kept turns remain; shorten their tool results too.
    const recent = trimTurns(last);
    const kept = [
      ...(summary ? [summary] : []),
      ...trimmed.slice(0, -1).flat(),
      ...recent.flat(),
    ];
    return {
      history: kept,
      compaction: { trimmedResults, summarizedTurns: 0 },
    };
  }

  const folded = trimmed.slice(0, -options.keepTurns);
  const recent = trimmed.slice(-options.keepTurns);
  const transcript = [
    summary ? contentText(summary) : "",
    ...folded.map(renderTurn),
  ]
    .filter(Boolean)
    .join("\n\n");
  const text = await summarize(transcript);
  return {
    history: [summaryItem(text), ...recent.flat()],
    compaction: { trimmedResults, summarizedTurns: folded.length },
  };
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
