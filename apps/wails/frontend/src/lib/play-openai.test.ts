import assert from "node:assert/strict";
import test from "node:test";

import { clearPlayOpenAIClient, configurePlayOpenAIClient, getPlayOpenAIClient, readPlaySpeechAudioBlob } from "./gizclaw/openai.ts";

test("Play OpenAI client sends chat completions through the injected fetch", async () => {
  let request: Request | undefined;
  const fetchImpl: typeof fetch = async (input, init) => {
    request = new Request(input, init);
    return new Response(
      JSON.stringify({
        choices: [{ finish_reason: "stop", index: 0, message: { content: "ok", role: "assistant" } }],
        created: 1,
        id: "chatcmpl-test",
        model: "model-test",
        object: "chat.completion",
      }),
      { headers: { "content-type": "application/json" }, status: 200 },
    );
  };
  configurePlayOpenAIClient(fetchImpl);

  const response = await getPlayOpenAIClient().chat.completions.create({
    messages: [{ content: "hello", role: "user" }],
    model: "model-test",
  });

  assert.equal(request?.url, "http://gizclaw/v1/chat/completions");
  assert.equal(response.choices[0]?.message.content, "ok");
  clearPlayOpenAIClient(fetchImpl);
  assert.throws(() => getPlayOpenAIClient(), /not connected/);
});

test("Play speech parser preserves the streamed Ogg audio type", async () => {
  const audio = new TextEncoder().encode("OggS\0test-audio");
  const body = [
    `data: ${JSON.stringify({ audio: Buffer.from(audio).toString("base64"), type: "speech.audio.delta" })}`,
    `data: ${JSON.stringify({ done: true, type: "speech.audio.done" })}`,
    "",
  ].join("\n");
  const response = new Response(body, { headers: { "content-type": "text/event-stream" } });

  const blob = await readPlaySpeechAudioBlob(response, "audio/mpeg");

  assert.equal(blob.type, "audio/ogg");
  assert.deepEqual(new Uint8Array(await blob.arrayBuffer()), audio);
});
