# Flowcraft Transformer

`pkgs/genx/transformers/flowcraft` wraps a Flowcraft Graph as a concurrently reusable `genx.Transformer`. It depends only on GenX and generic Stores, not on GizClaw Workspace, Workflow, AgentHost, Claw, or product Toolkit types.

## Construction

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
    Toolkit:      executableToolkit,
    MaxToolCalls: 32,

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

`Graph` is required and accepts LLM nodes, script nodes with inline `source`, and passthrough nodes. Scripts can operate on the Board and use runtime features such as match, but no Workspace is installed, so filesystem APIs are unavailable. `PublishNodes` explicitly selects which node assistant tokens can enter the output Stream.

An LLM node's `model` field is an alias such as `chat`. The Transformer resolves it internally as `Models.GenerateStream(ctx, "model/chat", modelContext)`; a Graph cannot supply a raw provider model ID or bypass the runtime alias.

The model adapter carries the GenX-defined max tokens, temperature, top-p, top-k, penalties, thinking, and extra fields. Flowcraft stop words and structured/image output without their existing typed path return an explicit error instead of applying provider-specific guesses.

Parallel Graph execution always uses the Flowcraft SDK defaults: up to 10 branches, three nesting levels, and `last_wins` merge. A Graph without a fork creates no extra branches. The Publisher buffers speculative candidates, exposes only the accepted branch, and drops cancelled branches.

## Stream lifecycle

Each constructed Transformer owns one ContextID. All `Transform(ctx, input)` calls during that Agent lifetime share the ContextID, History, Memory scope, and Board State. A configured `ContextID` restores the same History and State after reconstruction; an empty value generates an Agent-lifetime identity for standalone use. GizClaw's workflow Factory derives the stable value from the Workspace/Agent scope rather than exposing it in Workflow YAML. Concurrent Transform calls still own independent runs, input accumulators, output buffers, and cancellation.

`InitiativeOnReload` runs one empty-input Graph turn on the first `Transform` of a Transformer lifetime. `InitiativeOnceWhenEmpty` does so only when the configured conversation History is empty. Initiative is claimed once across concurrent attachments and later Transform calls.

Text input starts at BOS and is accumulated until the matching text EOS before the Graph runs. Each completed text turn produces a fresh output StreamID, BOS, streaming text, and EOS. Non-text content bypasses Flowcraft unchanged on its original route.

A new text BOS, whether control-only or text-bearing, cancels an unfinished prior turn, discards its unfinished input and unpulled output, and emits an EOS with an `interrupted` error after that turn reaches its persistence boundary. Because a control-only BOS has no MIME declaration, Flowcraft treats it eagerly as the next text turn; a non-text route that must not interrupt uses a MIME-bearing BOS. A control-only BOS left without text when input ends is dropped instead of creating an orphan output route. History and Memory contain only assistant text that crossed the final delivery observation boundary; a discarded or not-yet-delivered suffix is not recorded. An interruption with no delivered assistant text is not submitted to History or Memory. Otherwise the interrupted user/assistant History pair is stored and its assistant message carries an interruption data marker.

## Store boundaries

- `History` uses the caller-provided `logstore.MutableStore` and `HistoryScope` for ordered turns across one Agent lifetime. Agent-local memory is used when it is nil.
- `State` uses a caller-prefixed `kv.Store` for JSON-serializable Board variables. `response`, `usage`, `tool`, `tmp_*`, and `__*` variables are excluded.
- `Memory` uses the provider-neutral `memory.Store`. Every recall and observation uses the fixed caller-configured `MemoryScope`.

`RecallRenderer` and `ObservationBuilder` have package defaults and may be replaced. The default recall value is a `Relevant memory:` list. The default observation contains the user turn and the assistant text actually pulled by downstream, never Board variables as facts.

On top of that reusable default, the GizClaw workflow Factory handles public `memory.write.board_facts`: only explicitly named Board variables become `memory.FactCandidate` values, and the Flowcraft Memory Store ingests them directly as structured facts. When `save_conversation: false`, user and assistant turns are not persisted as a side effect.

With `ObserveWaitForCompletion=false`, EOS and the next turn wait for `Observe` acceptance but not for an asynchronous operation to materialize. Stores that implement `memory.AsyncOperationProcessor` materialize that operation in the background. When true, Memory must implement `memory.OperationWaiter`, and both the current EOS and next Graph turn wait for operation completion. The input pump continues reading in either mode and does not use downstream output as backpressure.

When `Toolkit` is non-nil, every LLM model context advertises its defensive function declarations. ToolCalls execute in model order, their JSON results are appended to the same model turn, and generation continues until the model returns no calls. Text produced before and after tool rounds remains streamable; ToolCall and ToolResult control data never enters the public GenX output.

`MaxToolCalls` is shared by all nodes in one `Transform` invocation. Zero uses 32, negative values are rejected, repeated call IDs fail within the invocation, and independent concurrent invocations may reuse the same provider call ID. Executor errors, invalid arguments, exhaustion, cancellation, and result serialization errors terminate only the affected invocation.
