import type { Model } from "@openai/agents-core";
import { OpenAIChatCompletionsModel } from "@openai/agents-openai";
import OpenAI from "openai";

export type GizClawModelOptions = {
  /** The node's OpenAI-compatible base URL, e.g. https://node.example.com/openai/v1. */
  baseURL: string;
  /** A GizClaw API key; its owner's RuntimeProfile selects the models. */
  apiKey: string;
  /** A RuntimeProfile model alias or model id, e.g. "llm". */
  model: string;
  fetch?: typeof fetch;
};

/**
 * Creates a Chat Completions model that talks to a GizClaw /openai/v1 node.
 * Tools run in the caller; GizClaw forwards declarations and calls only.
 */
export function createGizClawModel(options: GizClawModelOptions): Model {
  const client = new OpenAI({
    baseURL: options.baseURL.replace(/\/+$/, ""),
    apiKey: options.apiKey,
    // The console runs in the browser by design and holds the key the
    // operator configured; no other origin receives it.
    dangerouslyAllowBrowser: true,
    fetch: options.fetch,
  });
  return new OpenAIChatCompletionsModel(client, options.model);
}
