import {
  Usage,
  type AgentOutputItem,
  type Model,
  type ModelRequest,
  type ModelResponse,
} from "@openai/agents-core";

/**
 * One model step: tool calls to make, optionally with text said in the same
 * response, or the final reply.
 */
export type ScriptStep =
  | {
      say?: string;
      call: { name: string; arguments: Record<string, unknown> }[];
    }
  | { reply: string };

/**
 * A Model that replays a fixed script, one step per model call, across all
 * turns of a conversation. It exercises the real Agents SDK runner, tool
 * execution and history handling without a network.
 */
export class ScriptedModel implements Model {
  readonly requests: ModelRequest[] = [];
  private readonly steps: ScriptStep[];
  private next = 0;
  private callSequence = 0;

  constructor(steps: ScriptStep[]) {
    this.steps = steps;
  }

  get exhausted(): boolean {
    return this.next >= this.steps.length;
  }

  async getResponse(request: ModelRequest): Promise<ModelResponse> {
    this.requests.push(request);
    const step = this.steps[this.next++];
    if (!step)
      throw new Error(
        `the script has no step ${this.next} for this model call`,
      );
    return { usage: new Usage(), output: this.output(step) };
  }

  // The assistant runs non-streaming turns; a streamed call means the
  // session started using a code path the script cannot answer.
  getStreamedResponse(): AsyncIterable<never> {
    throw new Error("ScriptedModel does not script streamed responses");
  }

  private output(step: ScriptStep): AgentOutputItem[] {
    const text = "reply" in step ? step.reply : step.say;
    const output: AgentOutputItem[] = text
      ? [
          {
            type: "message",
            role: "assistant",
            status: "completed",
            content: [{ type: "output_text", text }],
          },
        ]
      : [];
    if ("reply" in step) return output;
    for (const call of step.call) {
      output.push({
        type: "function_call",
        callId: `call_${++this.callSequence}`,
        name: call.name,
        arguments: JSON.stringify(call.arguments),
        status: "completed",
      });
    }
    return output;
  }
}
