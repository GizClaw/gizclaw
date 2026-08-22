# DashScope Adapter

DashScope Adapter adapts a DashScope realtime multimodal session to `genx.Transformer` through `dashscoperealtime.Transformer`.

The public constructor requires both the client and an explicit model. It stores immutable provider options without opening a WebSocket; each concurrent `Transform` call creates its own session.

```go
transformer, err := dashscoperealtime.New(dashscoperealtime.Config{
    Client:       client,
    Model:        dashscope.ModelQwen35OmniFlashRealtime,
    ToolInvoker: runtimeTools,
    MaxToolCalls: 32,
})
```

`Model` is required and is never inferred by the adapter. Select a DashScope Qwen 3.5 Omni Realtime model when enabling function tools; legacy Qwen Omni Turbo Realtime and Qwen 3 Omni Flash Realtime models do not support provider Function Calling and are rejected when `ToolInvoker` is configured.

The default output voice follows the selected model family. Qwen 3.5 Omni Realtime uses `Tina`; the legacy Qwen Omni Turbo Realtime default remains `Chelsie`. An explicitly configured `Voice` is preserved.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `dashscoperealtime.Config` | Stores the client, realtime model, audio format, voice, instructions, turn detection, and optional ToolInvoker configuration. |
| `dashscoperealtime.New` | Creates a Transformer from typed Config without opening a connection. |
| `dashscoperealtime.Transformer.Transform` | Establishes an independent realtime session for each call, writes the input Stream to the provider, and returns the unified output Stream. |
| `dashscoperealtime.Stream` | Wraps the realtime output Stream that supports session updates. |

Provider session update and event name remain inside the Adapter; the caller only relies on GenX Stream and an explicit update contract.

## Function-tool continuation

When `ToolInvoker` is non-nil, each `Transform` resolves the current tool names, descriptions, and JSON Schemas before opening its provider session. DashScope function calls execute in provider order through `InvokeTool(name, arguments)`. Each raw JSON result is submitted with the original provider call ID, then `response.create` continues the same conversation. ToolCall and ToolResult control data remain internal and never enter the public GenX Stream.

Function declarations, function-call events, result submission, and continuation use the DashScope SDK's typed `FunctionTool`, event, `SubmitFunctionCallOutput`, and `CreateResponse` surfaces. The Adapter does not maintain a parallel raw event map or wire protocol.

The Transformer owns provider call IDs, ordering, continuation, duplicate-ID rejection, and the per-invocation `MaxToolCalls` budget. Zero uses 32 and negative values are rejected. A nil invoker explicitly configures an empty provider tool list. Independent concurrent `Transform` calls have separate call-ID sets and budgets, even when they share one invoker.

Resolution, invocation, invalid result JSON, result submission, continuation, cancellation, duplicate-ID, and budget errors terminate only the affected Transform. The injected invoker owns runtime resource lookup, authorization, argument validation, and executor dispatch; DashScope does not receive RuntimeProfile, Toolkit, or executor-registry internals.
