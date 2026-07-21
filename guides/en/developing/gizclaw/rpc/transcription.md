# Standalone Speech Transcription

`server.speech.transcribe` converts a bounded audio upload into one final transcript without creating or selecting a Workspace.

The request contains a RuntimeProfile `model_alias`, `content_type`, and optional `language`. The initial wire format is `audio/L16;rate=16000;channels=1`: signed 16-bit little-endian mono PCM at 16 kHz. After the typed request envelope, the Client sends incremental binary frames and request EOS. The Server forwards chunks through backpressure to the resolved ASR Transformer, then returns `SpeechTranscribeResponse` and response EOS.

One call owns one reliable Peer RPC service stream. It does not create an audio track, Media Channel, Peer connection, Workspace, history entry, or stored audio. Closing the stream or cancelling the context cancels provider work.

Server config owns operational limits:

```yaml
speech:
  transcription:
    max_audio_bytes: 2097152
    max_audio_duration: 60s
    request_timeout: 75s
```

The transcript wire limit is 8192 UTF-8 bytes. Invalid metadata is `INVALID_PARAMS`; unknown/dangling alias is `NOT_FOUND`; empty, malformed, unsupported, or over-limit audio is `BAD_REQUEST`; provider failures are redacted `INTERNAL_ERROR` responses.

Go `TranscribeSpeech`, JavaScript `transcribeSpeech`, and C `gzc_rpc_speech_transcribe_open/write/finish` expose incremental upload. Flutter receives the generated typed method and payload surface.
