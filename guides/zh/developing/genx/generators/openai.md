# OpenAI Adapter

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx)

OpenAI Adapter 由根包的 `OpenAIGenerator` 实现，把 OpenAI-compatible Chat Completions API 适配为 `genx.Generator`。

## 转换边界

- 将 `ModelContext` 的 prompts、messages、tools 和 model parameters 转为 OpenAI request。紧随 model text 的多个 tool call 合并为同一条携带全部调用的 assistant message；tool 声明的 `Parameters` 与 `Strict` 原样发送。
- 将 streaming text、binary content、tool call 和 finish reason 转为 `MessageChunk` 与 `State`。
- `Invoke` 优先使用 JSON Schema structured output，也可使用 function tool call。
- 将 token usage 转为统一的 `genx.Usage`。

OpenAI-compatible 只表示 provider protocol compatibility；credential、endpoint 和产品 model selection 由调用方提供。

`ModelParams.SetTemperature(0)` 显式设置零温度，`HasTemperature()` 表示是否配置了温度。JSON 中的 `"temperature": 0` 在解码、参数合并、再次编码和请求发送时保留；缺省或 `null` 不创建显式值。为兼容现有 Go 结构体字面量，直接写 `ModelParams{Temperature: 0}` 仍按未设置处理。显式温度覆盖 Generator 默认温度，其他未设置参数继续继承默认值。

流式响应的 `finish_reason: content_filter` 映射为 `StatusBlocked`，与 `length` 对应的
`StatusTruncated` 及网络/解码错误分开。上游未给出 refusal 内容时，错误会明确标记
`provider finish_reason=content_filter; refusal detail was not supplied`，不把空理由解释为
正常结束，也不猜测命中了哪条过滤策略。

## 音频输入

`OpenAIGenerator` 将同一个 user message 的音频 Blob 转为接口支持的输入：raw Opus、Ogg/Opus 和 signed 16-bit PCM 解码并转换为 16 kHz 单声道 WAV，MP3 与单个 WAV 原样传递。文本、业务 prompt、历史和 Tools 保持原有含义。

Generator 只执行调用方请求的模型生成，不追加转写请求。音频输入的 ASR 由 [AudioDock](../agentkit#audio-dock) 使用独立 ASR Transformer 完成，RuntimeProfile 分别选择对话 Model 与 ASR Model。

标准 Docker E2E 的 `eino-lite-native-audio.asr-roundtrip.giztest.yaml` 验证同一录音直接进入音频 Model，同时由 Profile 绑定的 SAUC 产出独立 transcript；Workflow 不提供转写 prompt。
