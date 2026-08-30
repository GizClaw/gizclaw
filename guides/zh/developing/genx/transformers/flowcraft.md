# Flowcraft Transformer

`pkgs/genx/transformers/flowcraft` 将 Flowcraft Graph 包装为可并发复用的 `genx.Transformer`。它只依赖 GenX 与通用 Store，不依赖 GizClaw Workspace、Workflow、AgentHost、Claw 或产品层 Toolkit 类型。

## 构造

```go
transformer, err := flowcraft.New(flowcraft.Config{
    ID:          "assistant",
    Name:        "Assistant",
    Description: "General assistant",
    ContextID:   "workspace/assistant",
    Graph:       graphDefinition,
    MaxIterations: 32,
    PublishNodes: []string{"answer"},
    Models:       runtimeGenerator,
    ToolInvoker:  runtimeTools,
    MaxToolCalls: 32,

    History: historyLogStore,
    Memory:  longTermMemoryStore,
    State:   boardStateStore,

    MemoryScope: memory.Scope{
        AppID: "runtime", UserID: "user", AgentID: "assistant",
    },
    MemoryContext: &memoryhook.ContextSettings{
        Query:  memoryhook.QuerySettings{CurrentMessage: true},
        Budget: memoryhook.BudgetSettings{MaxItems: 8},
        Output: "memory_items",
    },
    MemoryTurn: &memoryhook.TurnSettings{Channel: "answer"},
})
```

`Graph` 必须非空，并且接受 Flowcraft Core 0.2 的 `inference`、GizClaw 注册的 Match/Memory、使用内联 `source` 的 script 和 passthrough node。旧 `llm` discriminator 不再接受。Script 可以操作 Board，但没有 Workspace，因此文件读写 API 不可用。`PublishNodes` 明确指定哪些 node 的 assistant message channel 可以进入输出 Stream。

`inference` node 使用 Core 0.2 model identity，例如 `{"id":{"provider":"gizclaw","name":"chat"}}`；`name` 是 RuntimeProfile alias。Transformer 在内部把它解析为 `Models.GenerateStream(ctx, "model/chat", modelContext)`；Graph 不能直接填写 provider model ID 或绕过 Runtime 提供的 alias。`messages_channel` 选择该 node 写入的消息 channel，公开输出和 `memory.turn` 可以分别引用这个稳定名称。

模型适配传递 GenX 已定义的 max tokens、temperature、top-p、top-k、penalty、thinking 和 extra fields。Flowcraft 的 stop words，以及没有现有 typed path 的 structured/image output，会返回明确错误，不做 provider-specific 猜测。

## Match node

原生 `match` node 读取一个配置指定的 Board string，并写入一个 JSON-compatible 有序列表。它在 Transformer 构造期间编译共享的 `pkgs/genx/match` rules，并通过 `model/<alias>` 调用 `Models.GenerateStream`。

```yaml
- id: route
  type: match
  config:
    model: router
    input: input
    output: route_matches
    rules:
      - name: play_music
        vars:
          title:
            label: 歌曲名
            type: string
        patterns:
          - 我想听[title]
```

Config 只接受 `model`、`input`、`output` 和 `rules`。Alias 不能包含 `/`；alias 和 Board variable name 都必须非空且不能带首尾空格。输入缺失或 Go type 不是 `string` 时 node 直接失败，不做类型转换；空字符串仍是合法输入。

该 node 不发布 assistant token，也不应列入 `PublishNodes`。只有模型 stream 与 Match 解析全部成功后才写入输出；model、stream、parse 或 cancellation error 都不会发布 Board output。每个已返回的 model stream 只关闭一次。编译后的 Matcher 不可变，可以在独立 Board 间并发执行。

并行 Graph 始终开启 Flowcraft SDK 默认策略：最多 10 个 branch、最多 3 层嵌套、`last_wins` merge。Graph 本身没有 fork 时不会产生额外 branch。Publisher 缓存 speculative candidate，只输出最终 accept 的 branch，cancel 的 branch 不进入 GenX Stream。

## Stream 生命周期

每个已构造 Transformer 拥有一个 ContextID，同一 Agent lifetime 内的所有 `Transform(ctx, input)` 共用该 ContextID、History、Memory scope 与 Board State。显式配置 `ContextID` 时，重新构造后会继续使用相同的 History 与 State；留空时则为 standalone 使用场景生成 Agent-lifetime identity。GizClaw workflow Factory 会从 Workspace/Agent scope 派生稳定值，不要求 Workflow YAML 暴露该字段。同一个 Transformer 可以并发执行多个 Transform，各调用仍独立拥有 run、输入聚合、输出 buffer 与取消状态。

`InitiativeOnReload` 会在 Transformer 生命周期内第一次 `Transform` 时运行一次空输入 Graph turn；`InitiativeOnceWhenEmpty` 仅在已配置会话 History 为空时运行。并发 attach 和后续 `Transform` 共享一次 initiative claim，不会重复触发。

文本输入以 BOS 开始并持续聚合，直到对应文本 EOS 后才运行 Graph。每个完成的文本 turn 产生新的输出 StreamID、BOS、streaming text 和 EOS。非文本内容不进入 Flowcraft，按原 route 原样通过。

新的文本 BOS（无论是纯控制还是携带文本 Part）会取消尚未完成的上一轮，丢弃该轮尚未完成的输入和未 pull 的输出，并在该轮完成持久化边界后发送带 `interrupted` error 的 EOS。由于纯控制 BOS 不声明 MIME，Flowcraft 会立即把它视为下一轮文本输入；不应打断文本回复的非文本 route 必须使用携带 MIME Part 的 BOS。输入结束时仍没有文本的纯控制 BOS 会被丢弃，不会形成悬空输出 route。History 和 Memory 只记录已经跨过最终 delivery observation 边界的 assistant 文本；被删除或尚未真正交付的后缀不会被记录。没有交付 assistant 文本的中断不会提交到 History 或 Memory；否则保存被打断的 user/assistant History 对，并给 assistant message 添加 interruption data marker。

## Store 与 Memory hook 边界

- `History` 使用调用方提供的 `logstore.MutableStore`，按 `HistoryScope` 保存同一 Agent lifetime 内的有序对话。为空时使用 Agent-local memory。
- `State` 使用调用方已做好 prefix 的 `kv.Store`，保存可 JSON 序列化的 Board variables。`response`、`usage`、`tool`、`tmp_*` 和 `__*` 不持久化。
- `Memory` 使用 provider-neutral `memory.Store`。在 GizClaw Workflow 中，`MemoryScope` 由当前 Workspace runtime 的 App、User 与 Agent scope 派生，不能通过 Workflow JSON 或 YAML 覆盖；所有 recall 与 observe 都使用该固定 scope。上面的字面量仅演示 standalone 调用方如何显式提供 scope。Mem0 和 Flowcraft Memory 0.1.7 仍通过同一个 Store interface 接入。

`MemoryContext` 注册官方 `memory.context` prepare hook。其 query 必须选择 literal、Board variable 或当前输入消息之一，budget 和 min score 在调用 `memory.Store.Recall` 前生效；结果先写入配置的 item Board variable，可选 renderer 再写入单独的文本 Board variable。未配置时不注册 context hook。

`MemoryTurn` 注册官方 `memory.turn` commit hook。它从配置的 Core 0.2 message channel 读取已经交付的消息，并通过 `memory.Store.Observe` 提交；未指定 channel 时使用 Core 默认 conversation channel，未配置时不注册 turn hook。Provider operation 的原始错误不会进入公开 Flowcraft 错误文本。

GizClaw workflow Factory 会在这个 reusable 默认值之外处理公开配置中的 `memory.write.board_facts`：只有显式列出的 Board variables 会转换为 `memory.FactCandidate`，并由 Flowcraft Memory Store 直接写入结构化 fact；`save_conversation: false` 时不会顺带保存 user/assistant turns。

显式 `memory_observe` node 的 `wait_for_completion: false` 时，EOS 和下一轮只等待 `Observe` 接受数据，不等待异步 operation materialize；实现 `memory.AsyncOperationProcessor` 的 Store 会在后台完成该 operation。设为 `true` 时，Memory 必须实现 `memory.OperationWaiter`，当前 EOS 和下一轮 Graph 都等待 operation 完成。输入 pump 在两种模式下都继续读取，不依赖输出消费者提供背压。

`ToolInvoker` 非空时，每次 LLM model 调用都会通过 `ResolveTools` 取得可用函数的名称、说明和 schema。ToolCall 按模型给出的顺序通过 `InvokeTool(name, arguments)` 执行，JSON result 被追加到同一 model turn，再继续生成，直到模型不再返回调用。工具轮次前后的文本继续流式输出；ToolCall 与 ToolResult control data 不会进入公开 GenX output。

Flowcraft 不接收 RuntimeProfile、Toolkit policy、resource 或 Executor registry 细节。注入的 `ToolInvoker` 负责解析与执行；Flowcraft 只拥有 provider call ID、顺序、续跑和 `MaxToolCalls` guard。同一 `Transform` invocation 的所有 node 共用该额度：零值采用 32，负数非法；同一 invocation 内重复 call ID 会失败，而不同并发 invocation 可以复用相同 provider call ID。解析、执行、非法 result JSON、额度耗尽和取消错误只终止受影响的 invocation。
