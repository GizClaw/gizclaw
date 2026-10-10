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

流式响应的 `finish_reason: content_filter` 映射为 `StatusBlocked`，与 `length` 对应的
`StatusTruncated` 及网络/解码错误分开。上游未给出 refusal 内容时，错误会明确标记
`provider finish_reason=content_filter; refusal detail was not supplied`，不把空理由解释为
正常结束，也不猜测命中了哪条过滤策略。

## Doubao 音频输入

`pkgs/genx/generators/doubaochat` 包装 Volc 方舟 Doubao chat completions 的 `OpenAIGenerator`。Doubao Seed chat Model 接受音频输入，但 API 不返回独立的输入转写字段。适配合并每个 user message 的音频 Blob：raw Opus packet、Ogg/Opus 与 signed 16-bit PCM 解码为 16 kHz 单声道 WAV，MP3 与单个 WAV 原样传递。

最新 user message 含音频时，适配对同一 Model 并行发起两个独立请求。回复请求保留调用方原有的 prompts、history、Tools、CoT 和参数，只替换已转换的音频，不注入 ASR 指令。转写请求只包含当前音频与组件内置的纯转写提示，不带业务 prompt、history、Tools 或 CoT；它要求返回只有一个字符串字段的 JSON 对象 `{"transcript":"逐字转写"}`，空字符串表示没有语音。

对外仍是一条 `genx.Stream`。回复 chunk 原样透传，不解析、增加或删除业务正文中的 `<asr>`；转写结果通过 `genx.NewInputTranscriptChunk` 作为 `RoleUser`、`genx.InputTranscriptLabel` sideband 发布。两个请求独立启动和发布结果，回复首字不等待 ASR，ASR 也不等待回复完成；外层 stream 正常结束前会等待两者都完成。调用方不需要提供转写 prompt，也不需要改变可见回复格式。两次请求的上游用量分别记录，正常结束的 `State.Usage()` 返回两者合计。

转写请求最多等待 30 秒，设置 2048 completion tokens，最多读取 64 KiB 文本。缺少字段、重复或额外字段、非字符串值、无效 JSON 或上游错误都使外层 stream 明确失败，不把回复正文当作转写。任一路失败、调用方取消或关闭 stream 时，会取消另一请求并回收两条上游流；不发起额外恢复或重试请求。

纯文本请求原样转发；`Invoke` 只转换音频。peergenx 为 `support_text_only` 不为 true 的 Volc `chat_completions` Model 装配该适配，并用同一条件通过 `Service.AcceptsAudioInput` 回答“这个 Model 是否接受音频输入”；Eino factory 据此决定 [音频输入路径](/zh/developing/gizclaw/services/ai#eino-音频输入路径)。

标准 Docker E2E 的 `eino-lite-native-audio.asr-roundtrip.giztest.yaml` 使用 `doubao-lite-audio-chat`（`doubao-seed-2-1-lite-260915`）。对应 Workflow 不声明业务 prompt、`asr_model` 或回复 Voice；Giztest 先合成固定输入录音，再断言运行路径为 `AUDIO_INPUT_PATH_MODEL`、Lite 返回正确的独立 transcript 与非空回复，回复正文没有 ASR 标签。输入合成只用于准备录音，不参与识别；转写由 Lite 适配内部的独立请求完成。
