# DashScope Adapter

DashScope Adapter 通过 `dashscoperealtime.Transformer` 将 DashScope realtime multimodal session 适配为 `genx.Transformer`。

公共构造入口为 `dashscoperealtime.New(dashscoperealtime.Config{Client: client})`。Config 保存已解析的 DashScope client、model、voice、modalities、VAD 和 audio format 等不可变选项；constructor 不建立 WebSocket，每个并发 `Transform` 调用建立自己的 session。

```go
transformer, err := dashscoperealtime.New(dashscoperealtime.Config{
    Client:       client,
    Model:        dashscope.ModelQwen35OmniFlashRealtime,
    ToolInvoker: runtimeTools,
    MaxToolCalls: 32,
})
```

启用 function tools 时应选择 DashScope Qwen 3.5 Omni Realtime model；旧版 Qwen Omni Turbo Realtime 和 Qwen 3 Omni Flash Realtime model 不支持 provider Function Calling。配置 `ToolInvoker` 且 `Model` 为空时，constructor 自动选择 `ModelQwen35OmniFlashRealtime`；显式配置不支持的 model 会被拒绝。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `dashscoperealtime.Config` | 保存 client、realtime model、audio format、voice、instructions、turn detection 和可选 ToolInvoker 配置。 |
| `dashscoperealtime.New` | 使用类型化 Config 创建 Transformer；构造时不建立连接。 |
| `dashscoperealtime.Transformer.Transform` | 为每次调用建立独立 realtime session，将输入 Stream 写入 provider，并返回统一输出 Stream。 |
| `dashscoperealtime.Stream` | 包装支持 session update 的 realtime output Stream。 |

Provider session update 和 event name 留在 Adapter 内部；调用方只依赖 GenX Stream 与显式 update contract。

## Function-tool 续跑

`ToolInvoker` 非空时，每次 `Transform` 都会在打开 provider session 前解析当次可用工具的名称、说明和 JSON Schema。DashScope function call 按 provider 顺序通过 `InvokeTool(name, arguments)` 执行；每个 raw JSON result 使用原 provider call ID 提交，再通过 `response.create` 继续同一段会话。ToolCall 和 ToolResult control data 始终留在内部，不进入公开 GenX Stream。

函数声明、function-call event、result submission 和续跑都使用 DashScope SDK 的类型化 `FunctionTool`、event、`SubmitFunctionCallOutput` 与 `CreateResponse` 接口；Adapter 不维护另一套 raw event map 或 wire protocol。

Transformer 自己管理 provider call ID、顺序、续跑、重复 ID 拒绝和 invocation 级 `MaxToolCalls` 额度。零值采用 32，负数非法；nil invoker 会显式配置空 provider tool list。独立的并发 `Transform` 即使共用同一个 invoker，也各自拥有 call-ID set 和额度。

解析、执行、非法 result JSON、提交 result、续跑、取消、重复 ID 和额度耗尽错误只终止受影响的 Transform。注入的 invoker 负责 runtime resource lookup、权限、参数校验和 Executor dispatch；DashScope 不接收 RuntimeProfile、Toolkit 或 executor registry 细节。
