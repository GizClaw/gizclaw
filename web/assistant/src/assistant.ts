import {
  Agent,
  run,
  setTracingDisabled,
  type AgentInputItem,
  type Model,
  type RunItem,
} from "@openai/agents-core";

import { ASSISTANT_APIS } from "./apis.ts";
import {
  compactHistory,
  estimateTokens,
  type Compaction,
  type ContextOptions,
} from "./context.ts";
import { ASSISTANT_INSTRUCTIONS } from "./instructions.ts";
import type { AssistantRuntime } from "./runtime.ts";
import { createTools, type ActionRecord } from "./tools.ts";

/** The whole context budget per turn when the configuration names none. */
export const DEFAULT_CONTEXT_TOKENS = 32_000;

export type AssistantOptions = {
  runtime: AssistantRuntime;
  model: Model;
  /** Model calls allowed per user message, tool rounds included. */
  maxTurns?: number;
  /** Clock for log time windows; defaults to Date.now. */
  now?: () => number;
  /** A history saved from an earlier session, restored as the context. */
  history?: AgentInputItem[];
  /**
   * The token budget per turn: instructions, tool declarations and history.
   * History beyond it is compacted before the next message is sent.
   */
  contextTokens?: number;
  /** Most recent turns kept verbatim when older ones are summarized. */
  keepTurns?: number;
};

export type AssistantTurn = {
  reply: string;
  actions: ActionRecord[];
  /** Set when older history was trimmed or summarized before this turn. */
  compaction?: Compaction;
};

export type Assistant = {
  /**
   * Sends one user message and resolves after the assistant has finished
   * acting and replying. Aborting stops the turn; actions already taken are
   * read-only and stay recorded on the thrown error.
   */
  send(
    message: string,
    options?: { signal?: AbortSignal },
  ): Promise<AssistantTurn>;
  /** The context to save; restoring it with options.history resumes here. */
  history(): AgentInputItem[];
  /** Forgets the conversation history. */
  reset(): void;
};

/** A failed turn, carrying the actions taken before it failed. */
export class AssistantTurnError extends Error {
  readonly actions: ActionRecord[];

  constructor(cause: unknown, actions: ActionRecord[]) {
    super(cause instanceof Error ? cause.message : String(cause), { cause });
    this.name = "AssistantTurnError";
    this.actions = actions;
  }
}

const SUMMARY_INSTRUCTIONS = `你负责压缩诊断助手的对话记录。把给出的记录总结成一段中文摘要，供助手在后续对话里继续使用：
- 保留用户关心的设备、节点（名称和公钥）、问题、已经查到的关键事实（错误码、次数、数值、时间）、得出的结论和已经做过的跳转。
- 去掉寒暄和重复内容，不要编造记录里没有的信息。
- 只输出摘要本身。`;

export function createAssistant(options: AssistantOptions): Assistant {
  // Traces would otherwise be exported to the OpenAI platform.
  setTracingDisabled(true);
  let actions: ActionRecord[] = [];
  const agent = new Agent({
    name: "GizClaw 诊断助手",
    instructions: ASSISTANT_INSTRUCTIONS,
    model: options.model,
    tools: createTools({
      runtime: options.runtime,
      record: (action) => actions.push(action),
      now: options.now ?? Date.now,
    }),
  });
  const summarizer = new Agent({
    name: "对话摘要",
    instructions: SUMMARY_INSTRUCTIONS,
    model: options.model,
  });
  // Instructions and tool declarations are sent with every turn; the rest of
  // the budget belongs to the history.
  const overhead =
    estimateTokens(ASSISTANT_INSTRUCTIONS) +
    estimateTokens(ASSISTANT_APIS.map((api) => api.description));
  const context: ContextOptions = {
    maxTokens: Math.max(
      2_000,
      (options.contextTokens ?? DEFAULT_CONTEXT_TOKENS) - overhead,
    ),
    keepTurns: options.keepTurns ?? 4,
    trimChars: 1_500,
  };
  let history: AgentInputItem[] = [...(options.history ?? [])];
  let busy = false;

  const summarize = async (transcript: string, signal?: AbortSignal) => {
    const result = await run(summarizer, transcript, { maxTurns: 1, signal });
    return String(result.finalOutput ?? "");
  };

  return {
    async send(message, sendOptions) {
      if (busy) {
        throw new Error(
          "the assistant is still answering the previous message",
        );
      }
      busy = true;
      actions = [];
      const turnActions = actions;
      try {
        const compacted = await compactHistory(
          history,
          message,
          context,
          (transcript) => summarize(transcript, sendOptions?.signal),
        );
        history = compacted.history;
        const result = await run(
          agent,
          [...history, { role: "user", content: message }],
          { maxTurns: options.maxTurns ?? 12, signal: sendOptions?.signal },
        );
        history = result.history;
        const turn: AssistantTurn = {
          reply: turnReply(result.newItems),
          actions: turnActions,
        };
        if (compacted.compaction) turn.compaction = compacted.compaction;
        return turn;
      } catch (cause) {
        throw new AssistantTurnError(cause, turnActions);
      } finally {
        busy = false;
      }
    },
    history() {
      return structuredClone(history);
    },
    reset() {
      history = [];
    },
  };
}

// A model often explains its findings in the same response that calls a
// tool, then ends with a short confirmation; the reply keeps every text the
// assistant produced during the turn, in order, not just the last message.
function turnReply(items: RunItem[]): string {
  const texts: string[] = [];
  for (const item of items) {
    if (item.type !== "message_output_item") continue;
    for (const part of item.rawItem.content) {
      if (part.type === "output_text" && part.text.trim() !== "") {
        texts.push(part.text.trim());
      }
    }
  }
  return texts.join("\n\n");
}
