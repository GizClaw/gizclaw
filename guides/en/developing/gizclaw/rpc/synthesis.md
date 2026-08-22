# Standalone Speech Synthesis

`server.speech.synthesize` returns synthesized audio data without playing it on the Peer and without a Workspace.

The request contains a scoped RuntimeProfile `voice_name`, up to 4096 UTF-8 bytes of text, and one to eight accepted MIME types. The Server resolves the name to the canonical Voice, Model, tenant, and credential internally. Before binary audio, it returns `SpeechSynthesizeResponse` with the selected `content_type` and optional sample rate/channels. Binary frames are transport chunks, not codec packet boundaries, and response EOS terminates the stream.

The selected format is the first accepted MIME type that the Voice provider can deliver:

| Provider | `audio/ogg` (Ogg/Opus) | `audio/mpeg` | `audio/pcm` | `audio/flac` | `audio/wav` |
| --- | --- | --- | --- | --- | --- |
| Volc | native | native | native, `sample_rate_hz`/`channels` reported | - | - |
| MiniMax | 16 kHz mono, PCM transcoded on the Server (Volc parity) | native | native, `sample_rate_hz`/`channels` reported | native | native |

MiniMax offers no Opus container, so for `audio/ogg` the Server requests 16 kHz mono PCM from MiniMax and encodes Ogg/Opus itself at the same 16 kHz mono the Volc `ogg_opus` path uses (each synthesized segment is a complete Ogg logical bitstream with a distinct serial, so concatenated segments form a valid chained stream). The response carries `content_type: audio/ogg` without raw decoding metadata, exactly like the Volc path, so a device or Giztest `peer_stream` document cannot tell the providers apart. Any other media type, or a list without a deliverable type, is `BAD_REQUEST`.

The output remains backpressured from the TTS Transformer through the RPC writer to the Client reader. The Server does not buffer the full output, create a media track, call `server.run.say`, write history, or create a Workspace.

Server config owns operational limits:

```yaml
speech:
  synthesis:
    max_text_bytes: 4096
    max_output_bytes: 4194304
    request_timeout: 120s
```

Invalid metadata is `INVALID_PARAMS`; an unknown or dangling name is `NOT_FOUND`; unsupported or duplicate MIME types and invalid text are `BAD_REQUEST`; provider failures before metadata are redacted `INTERNAL_ERROR` responses. Failure after metadata terminates the stream abnormally, so a Client must not treat partial audio as complete.

Go `SynthesizeSpeech`, JavaScript `synthesizeSpeech`, and C `gzc_rpc_speech_synthesize` expose audio incrementally. Flutter receives the generated typed method and payload surface.
