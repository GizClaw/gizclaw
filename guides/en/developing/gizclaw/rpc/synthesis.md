# Standalone Speech Synthesis

`server.speech.synthesize` returns synthesized audio data without playing it on the Peer and without a Workspace.

The request contains a RuntimeProfile `voice_alias`, up to 4096 UTF-8 bytes of text, and one to eight accepted MIME types. The Server resolves the alias to the canonical Voice, Model, tenant, and credential internally. Before binary audio, it returns `SpeechSynthesizeResponse` with the selected `content_type` and optional sample rate/channels. Binary frames are transport chunks, not codec packet boundaries, and response EOS terminates the stream.

The output remains backpressured from the TTS Transformer through the RPC writer to the Client reader. The Server does not buffer the full output, create a media track, call `server.run.say`, write history, or create a Workspace.

Server config owns operational limits:

```yaml
speech:
  synthesis:
    max_text_bytes: 4096
    max_output_bytes: 4194304
    request_timeout: 120s
```

Invalid metadata is `INVALID_PARAMS`; unknown/dangling alias is `NOT_FOUND`; unsupported or duplicate MIME types and invalid text are `BAD_REQUEST`; provider failures before metadata are redacted `INTERNAL_ERROR` responses. Failure after metadata terminates the stream abnormally, so a Client must not treat partial audio as complete.

Go `SynthesizeSpeech`, JavaScript `synthesizeSpeech`, and C `gzc_rpc_speech_synthesize` expose audio incrementally. Flutter receives the generated typed method and payload surface.
