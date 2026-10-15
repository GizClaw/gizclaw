# MiniMax Adapter

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/minimaxtts)

The `minimaxtts` package adapts MiniMax speech synthesis to the GenX Transformer contract.

```go
transformer, err := minimaxtts.New(minimaxtts.Config{
    Client:  client,
    Model:   "speech-2.6-turbo",
    VoiceID: "female-shaonv",
})
```

`Config` stores the immutable client, model, voice, speed, volume, pitch, emotion, format, sample rate, bitrate, and `LanguageBoost` settings. `New` requires an explicit non-whitespace `Model`, validates the client and voice, and does not open a connection. It never substitutes `speech-2.6-hd` or another provider model. Each `Transform` call owns its Stream lifecycle and provider request state, so one configured Transformer supports concurrent calls.

`Config.Transport` defaults to `minimaxtts.TransportHTTP`. It receives incremental HTTP/SSE audio from `/v1/t2a_v2` and reuses the HTTP transport connection pool. The default path uses [shared client segmentation and audio-budget batching](./stream-processing#client-segmentation-and-batching): submit the first segment promptly, then combine later complete segments within the preceding audio's remaining playback time.

Explicit `minimaxtts.TransportBidi` sends incremental text to `/ws/v1/t2a_v2_bidi` and lets MiniMax buffer sentences. The shared native text path manages lexical cleanup, UTF-8 state, routes, message boundaries, backpressure, and completion without a client sentence segmenter or synthesis batch. It preserves newlines and sends whitespace with adjacent text because the provider discards whitespace-only events. Text exceeding 10,000 runes is split only to meet the wire limit.

On `MessageEnd`, bidi sends `task_flush` and waits for `task_flushed` and delivery of earlier audio before publishing the message boundary; later messages reuse the session. MessageID, role, name, or label changes also flush before switching identity. Only text EOS or normal input EOF sends `task_finish`, after which the reader drains remaining audio through `task_finished`. Neither `is_final` nor `sentence_end` ends the GenX route. Each Transform owns its session; cancellation and failures release IO and blocked delivery before joining workers. A ping every 40 seconds keeps an idle session active.

Bidi uses the same PCM-to-Ogg encoder. Provider sentences within a message share one Ogg logical stream; flush seals it and the next message receives a new serial. Reported `usage_characters` values are recorded as actual usage without estimating from input text.

Explicit `minimaxtts.TransportWebSocket` uses the ordinary WS protocol with client segmentation and batching. `genx.WithTTSBatching` controls HTTP and ordinary WS, while bidi keeps server sentence buffering. Construction rejects unknown transports. The GizClaw builder passes its configured HTTP client to the SDK; WS upgrades honor that client's transport and proxy too.

HTTP enables `stream_options.exclude_aggregated_audio`; the SDK also separates terminal aggregate audio from incremental output. After a terminal event, `Close` drains at most 64 KiB for up to 100 ms so a normal chunked response can return to the connection pool. Aborts close immediately. EOF before a terminal event is an error, and emitted prefixes are never retried or replayed. Default streaming HTTP lifetime follows context, while ordinary JSON requests have a 30-second timeout. An explicit `HTTPClient.Timeout` remains authoritative.

`GIZCLAW_TTS_DEBUG` enables `minimax tts: first audio timing`, covering connection reuse, DNS/TCP/TLS, response headers, provider first audio, and local encoding/enqueue time. HTTP and ordinary WS report per request; bidi reports per session. A value of `-1` means unobserved. WS timings include task startup, and bidi can include server buffering. Encoded headers or silence are not audible first-word measurements. Logs exclude text, authentication headers, and audio.

`LanguageBoost` is sent unchanged as MiniMax `language_boost`, for example `Japanese` or `French`; when empty the field is omitted and MiniMax infers the language from the text. `minimaxtts.LanguageBoost(code)` maps ISO 639-1 codes such as `ja`, `fr`, or `es-MX` (and the legacy `jp`) to MiniMax values and returns an empty string for an empty or unknown code. When GizClaw resolves a MiniMax Voice, the Voice pattern `language` parameter (for example `voice/<alias>?language=ja`) goes through that mapping into `LanguageBoost`. AST Translate passes its target language this way when it speaks through an external Voice; `auto` language pairs pass none.

The synthesis start log includes the effective model, voice ID, and language_boost as structured fields. It does not include synthesis text, credentials, audio, or raw provider payloads.

GizClaw MiniMax Voice resources must provide `provider_data.model`. Voice get/list responses preserve that configured value; a stored Voice without it remains readable but cannot be used to build a Transformer.

MiniMax TTS is a non-agent Stream-to-Stream Transformer and has no Toolkit configuration surface.
