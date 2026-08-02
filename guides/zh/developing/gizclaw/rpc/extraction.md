# 独立流式语音结构化提取

`server.speech.extract` 把一次有界音频上传转写成文本，再按调用方提供的 JSON Schema 返回结构化 JSON；它不创建或选择 Workspace。

Request 包含当前 RuntimeProfile 中 scoped `asr_model_name` 与 `extract_model_name`、`content_type`、可选 `language`、`schema_json` 和可选 `instruction`。两个 name 分别解析为 `Model(kind=asr)` 与 `Model(kind=llm)`；系统不会创建新的 Extract Model resource kind。初始音频格式是 `audio/L16;rate=16000;channels=1`。

Client 在 typed request envelope 后增量发送 binary audio frames 和 request EOS。Server 先通过 ASR Transformer 生成 transcript，再以 provider-neutral `genx.Generator.Invoke` 调用 LLM，并附带名为 `extract` 的 server-owned function tool。Provider 可以使用原生 tool call 或 schema-constrained output，但 Server 始终本地校验最终 JSON，并返回 `SpeechExtractResponse.transcript` 和规范化后的 `result_json`。

`schema_json` 是 UTF-8 JSON Schema，root 必须是 object schema。Server 接受 `jsonschema-go` 支持的 draft-07 或 2020-12 schema，不加载外部 `$ref`。下面同时是默认值和不可提高的固定上限；部署配置只能降低这些限制：

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

Transcript wire 上限是 8192 UTF-8 bytes。非法 RPC metadata 返回 `INVALID_PARAMS`；未知或 dangling name 返回 `NOT_FOUND`；非法 schema、instruction 或 audio 返回 `BAD_REQUEST`；provider failure、超时、缺少 `extract` tool call 或不符合 schema 的结果返回脱敏后的 `INTERNAL_ERROR`。

Go `ExtractSpeech`、JavaScript `extractSpeech` 与 C `gzc_rpc_speech_extract_open/write/finish` 提供增量上传；Flutter 提供生成后的 typed method 与 payload surface。
