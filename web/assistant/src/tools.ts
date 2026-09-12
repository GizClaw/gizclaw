import { tool, type FunctionTool } from "@openai/agents-core";
import { z } from "zod";

import { ApiInputError, ASSISTANT_APIS, type ApiDefinition } from "./apis.ts";
import { SourceError, type AssistantRuntime } from "./runtime.ts";

/** One tool call made during a turn, for the UI and for tests. */
export type ActionRecord = {
  tool: string;
  arguments: unknown;
  result?: unknown;
  error?: { code: string; message: string };
};

export type ToolContext = {
  runtime: AssistantRuntime;
  record(action: ActionRecord): void;
  now(): number;
};

/** One tool per catalog entry, in catalog order. */
export function createTools(context: ToolContext): FunctionTool[] {
  return ASSISTANT_APIS.map((api) => toTool(api, context));
}

// Tools declare plain JSON Schema instead of strict schemas: not every
// RuntimeProfile model enforces strict schemas, and Gemini rejects them.
// Arguments are validated here with the same zod schema. Optional arguments
// are nullish because models often send null for an omitted value.
function toTool(api: ApiDefinition, context: ToolContext): FunctionTool {
  const json = z.toJSONSchema(api.params, { target: "draft-7" }) as {
    properties?: Record<string, unknown>;
    required?: string[];
  };
  const parameters = {
    type: "object",
    properties: json.properties ?? {},
    required: json.required ?? [],
    additionalProperties: true,
  };
  return tool({
    name: api.tool,
    description: api.description,
    strict: false,
    // The SDK does not export its non-strict JSON Schema type; the shape above
    // is exactly JsonObjectSchemaNonStrict.
    parameters: parameters as never,
    execute: async (raw: unknown) => {
      const parsed = api.params.safeParse(raw);
      if (!parsed.success) {
        const error = {
          code: "invalid_arguments",
          message: z.prettifyError(parsed.error),
        };
        context.record({ tool: api.tool, arguments: raw, error });
        return { error };
      }
      try {
        const result = await api.run(context.runtime, parsed.data, context);
        context.record({ tool: api.tool, arguments: parsed.data, result });
        return result;
      } catch (cause) {
        const error = toolError(cause);
        context.record({ tool: api.tool, arguments: parsed.data, error });
        return { error };
      }
    },
  });
}

function toolError(cause: unknown): { code: string; message: string } {
  if (cause instanceof SourceError)
    return { code: cause.code, message: cause.message };
  if (cause instanceof ApiInputError)
    return { code: "invalid_arguments", message: cause.message };
  return {
    code: "source_failed",
    message: cause instanceof Error ? cause.message : String(cause),
  };
}
