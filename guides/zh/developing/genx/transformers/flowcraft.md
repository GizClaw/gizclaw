# Flowcraft Transformer

`pkgs/genx/transformers/flowcraft` 将 Flowcraft Graph 包装为可并发复用的 `genx.Transformer`。它只依赖 GenX 与通用 Store，不依赖 GizClaw Workspace、Workflow、AgentHost、Claw 或 Toolkit。

## 构造

```go
transformer, err := flowcraft.New(flowcraft.Config{
    ID:          "assistant",
    Name:        "Assistant",
    Description: "General assistant",
    Graph:       graphDefinition,
    MaxIterations: 32,
    PublishNodes: []string{"answer"},
    Models:       runtimeGenerator,

    History: historyLogStore,
    Memory:  longTermMemoryStore,
    State:   boardStateStore,

    MemoryScope: "runtime/user/assistant",
    RecallProfiles: []flowcraft.MemoryRecallProfile{{
        BoardVariable: "relevant_memory",
        Limit:         8,
    }},
    ObserveEnabled:           true,
    ObserveWaitForCompletion: false,
})
```

`Graph` 必须非空，并且只接受 LLM node 与使用内联 `source` 的 script node。Script 可以操作 Board 和使用 match 等运行时能力，但没有 Workspace，因此文件读写 API 不可用。`PublishNodes` 明确指定哪些 node 的 assistant token 可以进入输出 Stream。

LLM node 的 `model` 字段填写 alias，例如 `chat`。Transformer 在内部把它解析为 `Models.GenerateStream(ctx, "model/chat", modelContext)`；Graph 不能直接填写 provider model ID 或绕过 Runtime 提供的 alias。

模型适配传递 GenX 已定义的 max tokens、temperature、top-p、top-k、penalty、thinking 和 extra fields。Flowcraft 的 stop words、structured/image output 与 ToolCall 没有对应的通用 GenX text contract，因此会返回明确错误，不做 provider-specific 猜测。

并行 Graph 始终开启 Flowcraft SDK 默认策略：最多 10 个 branch、最多 3 层嵌套、`last_wins` merge。Graph 本身没有 fork 时不会产生额外 branch。Publisher 缓存 speculative candidate，只输出最终 accept 的 branch，cancel 的 branch 不进入 GenX Stream。

## Stream 生命周期

每次 `Transform(ctx, input)` 创建一个内部 ContextID。这个 ID 只在该调用内关联多个有序 turn；不会进入 public API、不会跨进程或跨 Transform 恢复。同一个 Transformer 可以并发执行多个 Transform，各调用拥有独立的 ContextID、run、输入聚合和输出 buffer。

文本输入以 BOS 开始并持续聚合，直到对应文本 EOS 后才运行 Graph。每个完成的文本 turn 产生新的输出 StreamID、BOS、streaming text 和 EOS。非文本内容不进入 Flowcraft，按原 route 原样通过。

新的文本 BOS 会取消尚未完成的上一轮，删除该轮未 pull 的输出，并在该轮完成持久化边界后发送带 `interrupted` error 的 EOS。History 和 Memory 只记录已经跨过输出 `Next()` 的 assistant 文本；被删除的未 pull 后缀不会被记录。被打断的 History assistant message 带 interruption data marker。

## Store 边界

- `History` 使用调用方提供的 `logstore.MutableStore`，保存同一 Transform 内的有序对话。为空时使用 invocation-local memory。
- `State` 使用调用方已做好 prefix 的 `kv.Store`，保存可 JSON 序列化的 Board variables。`response`、`usage`、`tool`、`tmp_*` 和 `__*` 不持久化。
- `Memory` 使用 provider-neutral `memory.Store`。`MemoryScope` 由调用方固定配置，所有 recall 与 observe 都使用同一 scope。

`RecallRenderer` 和 `ObservationBuilder` 都有 package 默认实现，也可以替换。默认 recall 文本写成 `Relevant memory:` 列表；默认 observation 只包含 user turn 和实际 pull 的 assistant turn，不把 Board variables 当作 fact。

`ObserveWaitForCompletion=false` 时，EOS 和下一轮只等待 `Observe` 接受数据，不等待异步 operation materialize；后续失败由 Memory Store 自己管理。设为 `true` 时，Memory 必须实现 `memory.OperationWaiter`，当前 EOS 和下一轮 Graph 都等待 operation 完成。输入 pump 在两种模式下都继续读取，不依赖输出消费者提供背压。

Toolkit continuation 不属于这个 Transformer 的当前 contract。
