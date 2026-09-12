import {
  Agent,
  run,
  setTracingDisabled,
  type AgentInputItem,
  type Model,
  type RunItem,
} from "@openai/agents-core";

import { ASSISTANT_INSTRUCTIONS } from "./instructions.ts";
import type { AssistantRuntime } from "./runtime.ts";
import { createTools, type ActionRecord } from "./tools.ts";

export type AssistantOptions = {
  runtime: AssistantRuntime;
  model: Model;
  /** Model calls allowed per user message, tool rounds included. */
  maxTurns?: number;
  /** Clock for log time windows; defaults to Date.now. */
  now?: () => number;
};

export type AssistantTurn = {
  reply: string;
  actions: ActionRecord[];
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
  let history: AgentInputItem[] = [];
  let busy = false;

  return {
    async send(message, sendOptions) {
      if (busy)
        throw new Error(
          "the assistant is still answering the previous message",
        );
      busy = true;
      actions = [];
      const turnActions = actions;
      try {
        const result = await run(
          agent,
          [...history, { role: "user", content: message }],
          {
            maxTurns: options.maxTurns ?? 12,
            signal: sendOptions?.signal,
          },
        );
        history = result.history;
        return { reply: turnReply(result.newItems), actions: turnActions };
      } catch (cause) {
        throw new AssistantTurnError(cause, turnActions);
      } finally {
        busy = false;
      }
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
