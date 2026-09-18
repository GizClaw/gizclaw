# DashScope Adapter

DashScope Adapter 通过 `dashscoperealtime.Transformer` 将 DashScope realtime multimodal session 适配为 `genx.Transformer`。

公共 constructor 要求同时传入 client 和显式 model。Config 保存已解析的 DashScope client、model、voice、modalities、VAD 和 audio format 等不可变选项；constructor 不建立 WebSocket，每个并发 `Transform` 调用建立自己的 session。

```go
transformer, err := dashscoperealtime.New(dashscoperealtime.Config{
    Client:       client,
    Model:        dashscope.ModelQwen35OmniFlashRealtime,
    ToolInvoker: runtimeTools,
    MaxToolCalls: 32,
})
```

`Model` 是必填项，Adapter 不会推断默认值。启用 function tools 时应选择 DashScope Qwen 3.5 Omni Realtime model；旧版 Qwen Omni Turbo Realtime 和 Qwen 3 Omni Flash Realtime model 不支持 provider Function Calling，并会在配置 `ToolInvoker` 时被拒绝。

默认输出音色随所选 model family 变化：Qwen 3.5 Omni Realtime 使用 `Tina`，旧版 Qwen Omni Turbo Realtime 仍使用 `Chelsie`。显式配置的 `Voice` 保持不变。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `dashscoperealtime.Config` | 保存 client、realtime model、audio format、voice、instructions、turn detection 和可选 ToolInvoker 配置。 |
| `dashscoperealtime.New` | 使用类型化 Config 创建 Transformer；构造时不建立连接。 |
| `dashscoperealtime.Transformer.Transform` | 为每次调用建立独立 realtime session，将输入 Stream 写入 provider，并返回统一输出 Stream。 |
| `dashscoperealtime.Stream` | 包装支持 session update 的 realtime output Stream。 |

Provider session update 和 event name 留在 Adapter 内部；调用方只依赖 GenX Stream 与显式 update contract。

## 初次建连

`Transform` 在读取 input、创建 output Stream 前完成 WebSocket 建连、等待
`session.created` 和首次 session update。此阶段的临时错误最多重试五次，退避依次为
100、200、400、800、1600 ms；每次失败都会关闭已创建的 session，最终失败保留原始错误。
等待和握手遵循调用方 context，取消会关闭尚在初始化的 session。

可恢复错误仅包括 HTTP 503、429、`ServiceBusy`、握手连接重置、broken pipe、EOF
与网络超时。鉴权、非法参数、其他 4xx 和未识别错误立即失败；结构化鉴权与参数错误
优先于临时 HTTP status。初始化成功后不重连、不重放输入或已产出的内容，运行期错误
仍然结束当前会话。该机制不适用于 Eino 首响 gate，也不改变其门槛或 Provider。

## 输出流关联

同一响应的语音转写文本与音频共享 `StreamID`，分别按 MIME 类型维护 BOS、数据和 EOS。独立的模型文本响应使用另一个 `StreamID`，避免文本结束事件提前关闭语音转写流；打断会关闭该响应的两条流。

## 语速

DashScope realtime 没有原生语速参数。`Config.SpeechRatePercent`（50..200，0 或 100 表示不变）由 GizClaw 从 Workspace `tts_speech_rate_percent` 传入，Transformer 用 [timestretch](../../audio/timestretch) 对每个回复的 PCM16 音频做保持音高的时间伸缩：音频到达即输出已确定的部分，`response.audio.done` 时 flush 余量后再发送 EOS；新回复的音频会丢弃被打断回复的残留状态。伸缩只支持 `pcm16` 输出（24 kHz 单声道），非默认语速与 `mp3`、`wav` 输出组合时 `New` 返回错误。

## Function-tool 续跑

`ToolInvoker` 非空时，每次 `Transform` 都会在打开 provider session 前解析当次可用工具的名称、说明和 JSON Schema。DashScope function call 按 provider 顺序通过 `InvokeTool(name, arguments)` 执行；每个 raw JSON result 使用原 provider call ID 提交，再通过 `response.create` 继续同一段会话。ToolCall 和 ToolResult control data 始终留在内部，不进入公开 GenX Stream。

函数声明、function-call event、result submission 和续跑都使用 DashScope SDK 的类型化 `FunctionTool`、event、`SubmitFunctionCallOutput` 与 `CreateResponse` 接口；Adapter 不维护另一套 raw event map 或 wire protocol。

Transformer 自己管理 provider call ID、顺序、续跑、重复 ID 拒绝和 invocation 级 `MaxToolCalls` 额度。零值采用 32，负数非法；nil invoker 会显式配置空 provider tool list。独立的并发 `Transform` 即使共用同一个 invoker，也各自拥有 call-ID set 和额度。

解析、执行、非法 result JSON、提交 result、续跑、取消、重复 ID 和额度耗尽错误只终止受影响的 Transform。注入的 invoker 负责 runtime resource lookup、权限、参数校验和 Executor dispatch；DashScope 不接收 RuntimeProfile、Toolkit 或 executor registry 细节。
