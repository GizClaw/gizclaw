# OpenAI Adapter

OpenAI Adapter 由根包的 `OpenAIGenerator` 实现，把 OpenAI-compatible Chat Completions API 适配为 `genx.Generator`。

## 转换边界

- 将 `ModelContext` 的 prompts、messages、tools 和 model parameters 转为 OpenAI request。紧随 model text 的多个 tool call 合并为同一条携带全部调用的 assistant message；tool 声明的 `Parameters` 与 `Strict` 原样发送。
- 将 streaming text、binary content、tool call 和 finish reason 转为 `MessageChunk` 与 `State`。
- `Invoke` 优先使用 JSON Schema structured output，也可使用 function tool call。
- 将 token usage 转为统一的 `genx.Usage`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`OpenAIGenerator`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx#OpenAIGenerator) | 保存 OpenAI client、model、生成参数和 capability flags，并实现 Generator。 |
| `OpenAIGenerator.GenerateStream` | 发起 streaming chat completion，并持续写入 GenX Stream。 |
| `OpenAIGenerator.Invoke` | 通过 structured output 或 tool call 生成 typed FuncCall arguments。 |
| [`FormatOpenAISchema`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx#FormatOpenAISchema) | 将通用 JSON Schema 规范化为 OpenAI structured-output schema。 |

OpenAI-compatible 只表示 provider protocol compatibility；credential、endpoint 和产品 model selection 由调用方提供。

## Doubao 音频输入

`pkgs/genx/generators/doubaochat` 包装 Volc 方舟 Doubao chat completions 的 `OpenAIGenerator`。Doubao Seed chat Model 接受音频输入，但 API 不返回输入转写。请求中 user message 的音频 Blob 会合并为一个 API 接受的 Blob：raw Opus packet、Ogg/Opus 与 signed 16-bit PCM 解码为 16 kHz 单声道 WAV，MP3 与单个 WAV 原样传递。最新 user message 含音频时，适配追加一条 system 指令：先写一整句回复，再单独一行 `<asr>逐字转写</asr>`，然后继续回复。这样首段回复不等待转写。适配从 streaming 文本中切掉第一段 `<asr>` segment（无论出现在开头、中间还是结尾），只暂扣可能是开标签前缀的尾部字符，并在该段闭合时输出一个 `genx.NewInputTranscriptChunk` 构造的 `RoleUser`、`genx.InputTranscriptLabel` chunk；转写以第一个闭合标签或换行结束，容忍 `</asr]` 这类错写，流结束时未闭合的段仍作为转写上报。回复中没有该段时照常输出回复并记录一条告警日志，不报告转写。纯文本请求原样转发；`Invoke` 只转换音频。peergenx 为 `support_text_only` 为 false 的 Volc `chat_completions` Model 装配该适配。
