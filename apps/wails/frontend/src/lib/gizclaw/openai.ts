import OpenAI from "openai";

const openAIAPIKey = "gizclaw-play";

let client: OpenAI | null = null;
let clientFetch: typeof fetch | null = null;

export function getPlayOpenAIClient(): OpenAI {
  if (client == null) {
    throw new Error("Play OpenAI service is not connected.");
  }
  return client;
}

export function configurePlayOpenAIClient(fetchImpl: typeof fetch): void {
  clientFetch = fetchImpl;
  client = new OpenAI({
    apiKey: openAIAPIKey,
    baseURL: "http://gizclaw/v1",
    dangerouslyAllowBrowser: true,
    fetch: fetchImpl,
    maxRetries: 1,
  });
}

export function clearPlayOpenAIClient(fetchImpl: typeof fetch): void {
  if (clientFetch !== fetchImpl) {
    return;
  }
  client = null;
  clientFetch = null;
}
