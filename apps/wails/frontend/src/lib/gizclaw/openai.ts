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

export async function readPlaySpeechAudioBlob(response: Response, fallbackContentType: string): Promise<Blob> {
  const contentType = response.headers.get("content-type") ?? "";
  if (!contentType.startsWith("text/event-stream")) {
    return response.blob();
  }
  if (response.body == null) {
    throw new Error("Speech stream response has no body");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const chunks: ArrayBuffer[] = [];
  let pending = "";
  let doneEvent = false;

  const processLine = (line: string) => {
    const trimmed = line.trim();
    if (trimmed === "" || !trimmed.startsWith("data:")) {
      return;
    }
    const data = trimmed.slice("data:".length).trim();
    const event = JSON.parse(data) as { audio?: string; done?: boolean; type?: string };
    switch (event.type) {
      case "speech.audio.delta":
        if (event.audio == null || event.audio === "") {
          throw new Error("Speech stream audio delta is empty");
        }
        chunks.push(base64ToArrayBuffer(event.audio));
        return;
      case "speech.audio.done":
        doneEvent = true;
        return;
      default:
        throw new Error(`Unexpected speech stream event: ${event.type ?? "unknown"}`);
    }
  };

  for (;;) {
    const { done, value } = await reader.read();
    pending += decoder.decode(value ?? new Uint8Array(), { stream: !done });
    for (;;) {
      const newline = pending.indexOf("\n");
      if (newline < 0) {
        break;
      }
      const line = pending.slice(0, newline);
      pending = pending.slice(newline + 1);
      processLine(line);
    }
    if (done) {
      break;
    }
  }
  if (pending.trim() !== "") {
    processLine(pending);
  }
  if (chunks.length === 0) {
    throw new Error("Speech stream returned no audio chunks");
  }
  if (!doneEvent) {
    throw new Error("Speech stream ended without done event");
  }
  return new Blob(chunks, { type: detectSpeechAudioContentType(chunks) ?? fallbackContentType });
}

function base64ToArrayBuffer(value: string): ArrayBuffer {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function detectSpeechAudioContentType(chunks: ArrayBuffer[]): string | undefined {
  const prefix = new Uint8Array(12);
  let length = 0;
  for (const chunk of chunks) {
    const bytes = new Uint8Array(chunk);
    const count = Math.min(bytes.length, prefix.length - length);
    prefix.set(bytes.subarray(0, count), length);
    length += count;
    if (length === prefix.length) {
      break;
    }
  }
  const ascii = String.fromCharCode(...prefix.subarray(0, length));
  if (ascii.startsWith("OggS")) {
    return "audio/ogg";
  }
  if (ascii.startsWith("ID3")) {
    return "audio/mpeg";
  }
  if (ascii.startsWith("fLaC")) {
    return "audio/flac";
  }
  if (ascii.startsWith("RIFF") && ascii.slice(8, 12) === "WAVE") {
    return "audio/wav";
  }
  return undefined;
}
