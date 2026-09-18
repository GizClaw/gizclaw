# MiniMax Adapter

The `minimaxtts` package adapts MiniMax speech synthesis to the GenX Transformer contract.

```go
transformer, err := minimaxtts.New(minimaxtts.Config{
    Client:  client,
    Model:   "speech-2.6-turbo",
    VoiceID: "female-shaonv",
})
```

`Config` stores the immutable client, model, voice, speed, volume, pitch, emotion, format, sample rate, bitrate, and `LanguageBoost` settings. `New` requires an explicit non-whitespace `Model`, validates the client and voice, and does not open a connection. It never substitutes `speech-2.6-hd` or another provider model. Each `Transform` call owns its Stream lifecycle and provider request state, so one configured Transformer supports concurrent calls.

`LanguageBoost` is sent unchanged as MiniMax `language_boost`, for example `Japanese` or `French`; when empty the field is omitted and MiniMax infers the language from the text. `minimaxtts.LanguageBoost(code)` maps ISO 639-1 codes such as `ja`, `fr`, or `es-MX` (and the legacy `jp`) to MiniMax values and returns an empty string for an empty or unknown code. When GizClaw resolves a MiniMax Voice, the Voice pattern `language` parameter (for example `voice/<alias>?language=ja`) goes through that mapping into `LanguageBoost`. AST Translate passes its target language this way when it speaks through an external Voice; `auto` language pairs pass none.

The synthesis start log includes the effective model, voice ID, and language_boost as structured fields. It does not include synthesis text, credentials, audio, or raw provider payloads.

GizClaw MiniMax Voice resources must provide `provider_data.model`. Voice get/list responses preserve that configured value; a stored Voice without it remains readable but cannot be used to build a Transformer.

MiniMax TTS is a non-agent Stream-to-Stream Transformer and has no Toolkit configuration surface.
