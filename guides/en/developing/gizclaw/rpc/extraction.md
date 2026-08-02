# Standalone Structured Speech Extraction

`server.speech.extract` transcribes one bounded audio upload and returns structured JSON constrained by the caller's JSON Schema. It does not create or select a Workspace.

The request contains scoped `asr_model_name` and `extract_model_name` values from the current RuntimeProfile, `content_type`, optional `language`, `schema_json`, and optional `instruction`. The names resolve to `Model(kind=asr)` and `Model(kind=llm)` respectively; the system does not introduce a separate Extract Model resource kind. The initial audio format is `audio/L16;rate=16000;channels=1`.

After the typed request envelope, the Client sends incremental binary audio frames and request EOS. The Server first produces a transcript through the ASR Transformer, then calls the LLM through provider-neutral `genx.Generator.Invoke` with a server-owned function tool named `extract`. A provider may use native tool calling or schema-constrained output, but the Server always validates the final JSON locally and returns `SpeechExtractResponse.transcript` plus canonical `result_json`.

`schema_json` is a UTF-8 JSON Schema whose root must describe an object. The Server accepts draft-07 or 2020-12 schemas supported by `jsonschema-go` and does not load external `$ref` targets. The following values are both defaults and fixed maxima; deployment config can only lower them:

```yaml
speech:
  extraction:
    max_schema_bytes: 16384
    max_schema_depth: 16
    max_schema_properties: 128
    max_instruction_bytes: 4096
    max_result_bytes: 16384
    request_timeout: 120s
```

The transcript wire limit is 8192 UTF-8 bytes. Invalid RPC metadata is `INVALID_PARAMS`; unknown or dangling names are `NOT_FOUND`; invalid schema, instruction, or audio is `BAD_REQUEST`; provider failures, timeouts, a missing `extract` tool call, or schema-invalid output are redacted `INTERNAL_ERROR` responses.

Go `ExtractSpeech`, JavaScript `extractSpeech`, and C `gzc_rpc_speech_extract_open/write/finish` expose incremental upload. Flutter receives the generated typed method and payload surface.
